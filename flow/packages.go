package flow

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PackageEdge is one package calling into another, weighted by how many call
// sites cross the boundary.
type PackageEdge struct {
	From string `json:"from" toon:"from"`
	To   string `json:"to" toon:"to"`
	// Calls is the number of call sites from From into To.
	Calls int `json:"calls" toon:"calls"`
	// Targets is how many distinct functions in To are called, which separates
	// "leans on one helper a lot" from "uses the whole package".
	Targets int `json:"targets" toon:"targets"`
}

// PackageNode is one package in the collapsed graph.
type PackageNode struct {
	Name    string `json:"name" toon:"name"`
	Funcs   int    `json:"funcs" toon:"funcs"`
	Entries int    `json:"entries" toon:"entries"`
	// Internal is the number of call sites that stay inside the package.
	Internal int `json:"internal" toon:"internal"`
	// Out lists outgoing edges, heaviest first.
	Out []PackageEdge `json:"out" toon:"out"`
}

// PackageFlow is the call graph collapsed to package granularity.
//
// The function-level graph answers "what calls what" but is far too large to
// read whole — a few hundred functions become a wall of text and an unreadable
// diagram. Collapsing to packages keeps the question most people are actually
// asking ("how does this system fan out, and what leans on what") while
// shrinking the answer by an order of magnitude.
type PackageFlow struct {
	Project      string        `json:"project" toon:"project"`
	Packages     []PackageNode `json:"packages" toon:"packages"`
	TotalFuncs   int           `json:"total_funcs" toon:"total_funcs"`
	TotalEntries int           `json:"total_entries" toon:"total_entries"`
	// Edges is the number of distinct package→package pairs.
	Edges int `json:"edges" toon:"edges"`
	// Degraded and Notes carry the parse-only caveat up from the call graph.
	Degraded bool     `json:"degraded" toon:"degraded"`
	Notes    []string `json:"notes,omitempty" toon:"notes,omitempty"`
}

// Leaves returns the packages that call no other package, sorted by name. They
// are the bottom of the stack — they may call plenty of their own functions,
// but nothing of anyone else's, so they can be read in isolation.
func (pf *PackageFlow) Leaves() []string {
	var out []string
	for _, p := range pf.Packages {
		if len(p.Out) == 0 {
			out = append(out, p.Name)
		}
	}
	sort.Strings(out)
	return out
}

// PackageGraph collapses a call graph to package granularity. Calls to nodes
// outside the graph (already filtered to the module) are ignored, so weights
// count only edges that resolve.
func PackageGraph(g *CallGraph, project string) *PackageFlow {
	pf := &PackageFlow{Project: project}
	if g == nil {
		return pf
	}
	pf.Degraded, pf.Notes = g.Degraded, g.Notes

	nodes := map[string]*PackageNode{}
	node := func(name string) *PackageNode {
		if p, ok := nodes[name]; ok {
			return p
		}
		p := &PackageNode{Name: name}
		nodes[name] = p
		return p
	}
	// Seed from the package list so a package with no calls at all still shows.
	for _, name := range g.Packages {
		node(name)
	}

	// edges[from][to] accumulates the weight and the distinct callees.
	edges := map[string]map[string]*PackageEdge{}
	targets := map[string]map[string]map[string]bool{}

	for _, n := range g.Nodes {
		from := node(n.Package)
		from.Funcs++
		pf.TotalFuncs++
		if n.IsEntry {
			from.Entries++
			pf.TotalEntries++
		}

		for _, callee := range n.Calls {
			cn, ok := g.Nodes[callee]
			if !ok {
				continue // out-of-module or unresolved
			}
			if cn.Package == n.Package {
				from.Internal++
				continue
			}
			if edges[n.Package] == nil {
				edges[n.Package] = map[string]*PackageEdge{}
				targets[n.Package] = map[string]map[string]bool{}
			}
			e, ok := edges[n.Package][cn.Package]
			if !ok {
				e = &PackageEdge{From: n.Package, To: cn.Package}
				edges[n.Package][cn.Package] = e
				targets[n.Package][cn.Package] = map[string]bool{}
			}
			e.Calls++
			targets[n.Package][cn.Package][callee] = true
		}
	}

	for from, tos := range edges {
		p := node(from)
		for to, e := range tos {
			e.Targets = len(targets[from][to])
			p.Out = append(p.Out, *e)
			pf.Edges++
		}
		sortEdges(p.Out)
	}

	pf.Packages = make([]PackageNode, 0, len(nodes))
	for _, p := range nodes {
		pf.Packages = append(pf.Packages, *p)
	}
	sortPackages(pf.Packages)
	return pf
}

// sortEdges orders a package's outgoing edges heaviest first, name as tiebreak.
func sortEdges(edges []PackageEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Calls != edges[j].Calls {
			return edges[i].Calls > edges[j].Calls
		}
		return edges[i].To < edges[j].To
	})
}

// sortPackages puts the packages that drive the most work first, so the top of
// the map is the part of the system worth reading.
func sortPackages(pkgs []PackageNode) {
	weight := func(p PackageNode) int {
		n := 0
		for _, e := range p.Out {
			n += e.Calls
		}
		return n
	}
	sort.Slice(pkgs, func(i, j int) bool {
		wi, wj := weight(pkgs[i]), weight(pkgs[j])
		if wi != wj {
			return wi > wj
		}
		if pkgs[i].Entries != pkgs[j].Entries {
			return pkgs[i].Entries > pkgs[j].Entries
		}
		return pkgs[i].Name < pkgs[j].Name
	})
}

// ─── Renderers ───────────────────────────────────────────────────────────────

// RenderPackageFlow draws the package map: each calling package with its
// outgoing edges, heaviest first, then the leaf packages as a single line.
// Targets sharing an edge weight are collapsed onto one row, which keeps a
// wide fan-out to three or four lines instead of a dozen.
func RenderPackageFlow(pf *PackageFlow) string {
	var sb strings.Builder

	name := pf.Project
	if name == "" {
		name = "project"
	}
	sb.WriteString(degradedBanner(&CallGraph{Degraded: pf.Degraded, Notes: pf.Notes}, "  "))
	fmt.Fprintf(&sb, "Flow map — %s\n\n", name)
	fmt.Fprintf(&sb, "  %d functions · %d packages · %d entry %s · %d package edges\n\n",
		pf.TotalFuncs, len(pf.Packages), pf.TotalEntries, plural(pf.TotalEntries, "point"), pf.Edges)

	if pf.TotalFuncs == 0 {
		sb.WriteString("  No calls resolved. Is this a Go module?\n")
		return sb.String()
	}

	// Widest weight decides the arrow column, so the arrows line up.
	width := 1
	for _, p := range pf.Packages {
		for _, e := range p.Out {
			if n := len(strconv.Itoa(e.Calls)); n > width {
				width = n
			}
		}
	}

	for _, p := range pf.Packages {
		if len(p.Out) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "  %s%s  %s\n", p.Name, entryMark(p), packageSummary(p))
		for _, row := range groupByWeight(p.Out) {
			fmt.Fprintf(&sb, "      ──%*d──▶  %s\n", width, row.calls, strings.Join(row.to, ", "))
		}
		sb.WriteString("\n")
	}

	if leaves := pf.Leaves(); len(leaves) > 0 {
		fmt.Fprintf(&sb, "  ── %d packages make no cross-package calls ──\n  %s\n",
			len(leaves), strings.Join(leaves, "  "))
	}
	return sb.String()
}

// entryMark flags packages that execution can start in.
func entryMark(p PackageNode) string {
	if p.Entries > 0 {
		return " 🚀"
	}
	return ""
}

// packageSummary is the per-package counts shown beside its name.
func packageSummary(p PackageNode) string {
	parts := []string{fmt.Sprintf("%d %s", p.Funcs, plural(p.Funcs, "func"))}
	if p.Entries > 0 {
		// "entry" pluralizes irregularly, so it can't go through plural().
		word := "entries"
		if p.Entries == 1 {
			word = "entry"
		}
		parts = append(parts, fmt.Sprintf("%d %s", p.Entries, word))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// weightRow is a set of packages called with the same number of call sites.
type weightRow struct {
	calls int
	to    []string
}

// groupByWeight collapses equally-weighted edges onto one row.
func groupByWeight(edges []PackageEdge) []weightRow {
	var rows []weightRow
	for _, e := range edges {
		if n := len(rows); n > 0 && rows[n-1].calls == e.Calls {
			rows[n-1].to = append(rows[n-1].to, e.To)
			continue
		}
		rows = append(rows, weightRow{calls: e.Calls, to: []string{e.To}})
	}
	return rows
}

// RenderPackageMermaid draws the package map as a Mermaid graph. At package
// granularity the node count is small enough that the diagram actually renders,
// which is not true of the function-level graph on any real repo.
func RenderPackageMermaid(pf *PackageFlow) string {
	var sb strings.Builder
	sb.WriteString("```mermaid\nflowchart LR\n")

	for _, p := range pf.Packages {
		id := mermaidID(p.Name)
		label := fmt.Sprintf("%s<br/>%d %s", p.Name, p.Funcs, plural(p.Funcs, "func"))
		if p.Entries > 0 {
			fmt.Fprintf(&sb, "    %s([\"%s\"])\n", id, label)
		} else {
			fmt.Fprintf(&sb, "    %s[\"%s\"]\n", id, label)
		}
	}
	sb.WriteString("\n")

	for _, p := range pf.Packages {
		for _, e := range p.Out {
			fmt.Fprintf(&sb, "    %s -->|%d| %s\n", mermaidID(e.From), e.Calls, mermaidID(e.To))
		}
	}
	sb.WriteString("```\n")
	return sb.String()
}
