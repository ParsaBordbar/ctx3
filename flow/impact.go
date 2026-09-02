package flow

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"sort"
	"strings"
)

// Caller is one function that reaches a target, at a given distance from it.
type Caller struct {
	Key     string `json:"key"     toon:"key"`
	Package string `json:"package" toon:"package"`
	Name    string `json:"name"    toon:"name"`
	File    string `json:"file"    toon:"file"`
	Line    int    `json:"line"    toon:"line"`
	Depth   int    `json:"depth"   toon:"depth"`
	IsEntry bool   `json:"isEntry" toon:"isEntry"`
}

// Impact is the blast radius of a symbol: everything that transitively calls it.
type Impact struct {
	Query    string   `json:"query"    toon:"query"`
	Targets  []string `json:"targets"  toon:"targets"`
	Callers  []Caller `json:"callers"  toon:"callers"`
	Packages []string `json:"packages" toon:"packages"`
	Entries  []string `json:"entries"  toon:"entries"`
	MaxDepth int      `json:"maxDepth" toon:"maxDepth"`
	// Degraded and Notes carry the parse-only caveat up from the call graph.
	// It matters more here than anywhere else: "no callers" is the answer a
	// reader acts on, and a name-resolved graph can miss a caller.
	Degraded bool     `json:"degraded" toon:"degraded"`
	Notes    []string `json:"notes,omitempty" toon:"notes,omitempty"`

	graph *CallGraph
}

// Reverse builds the caller index: callee key -> keys that call it.
func (g *CallGraph) Reverse() map[string][]string {
	rev := make(map[string][]string, len(g.Nodes))
	for key, node := range g.Nodes {
		for _, callee := range node.Calls {
			if _, ok := g.Nodes[callee]; !ok {
				continue
			}
			rev[callee] = append(rev[callee], key)
		}
	}
	for k := range rev {
		sort.Strings(rev[k])
		rev[k] = unique(rev[k])
	}
	return rev
}

// Match resolves a query to node keys: exact key first, then a `Name` or
// `Recv.Method` suffix, then a case-insensitive substring.
func (g *CallGraph) Match(query string) []string {
	if query == "" {
		return nil
	}
	if _, ok := g.Nodes[query]; ok {
		return []string{query}
	}

	var suffix, substr []string
	lower := strings.ToLower(query)
	for key, node := range g.Nodes {
		switch {
		case node.Name == query || strings.HasSuffix(node.Name, "."+query):
			suffix = append(suffix, key)
		case strings.Contains(strings.ToLower(key), lower):
			substr = append(substr, key)
		}
	}
	if len(suffix) > 0 {
		sort.Strings(suffix)
		return suffix
	}
	sort.Strings(substr)
	return substr
}

// Impacted returns everything that transitively calls query. maxDepth caps how
// many caller levels are walked (0 = unlimited).
func Impacted(g *CallGraph, query string, maxDepth int) (*Impact, error) {
	targets := g.Match(query)
	if len(targets) == 0 {
		return nil, fmt.Errorf("no function matching %q in the call graph", query)
	}

	rev := g.Reverse()
	imp := &Impact{
		Query: query, Targets: targets, MaxDepth: maxDepth, graph: g,
		Degraded: g.Degraded, Notes: g.Notes,
	}

	seen := map[string]bool{}
	for _, t := range targets {
		seen[t] = true
	}
	frontier := append([]string(nil), targets...)

	for depth := 1; len(frontier) > 0; depth++ {
		if maxDepth > 0 && depth > maxDepth {
			break
		}
		var next []string
		for _, key := range frontier {
			for _, caller := range rev[key] {
				if seen[caller] {
					continue
				}
				seen[caller] = true
				next = append(next, caller)

				node := g.Nodes[caller]
				imp.Callers = append(imp.Callers, Caller{
					Key:     caller,
					Package: node.Package,
					Name:    node.Name,
					File:    node.File,
					Line:    node.Line,
					Depth:   depth,
					IsEntry: node.IsEntry,
				})
			}
		}
		sort.Strings(next)
		frontier = next
	}

	pkgs := map[string]bool{}
	for _, t := range targets {
		if node := g.Nodes[t]; node != nil {
			pkgs[node.Package] = true
		}
	}
	for _, c := range imp.Callers {
		pkgs[c.Package] = true
		if c.IsEntry {
			imp.Entries = append(imp.Entries, c.Key)
		}
	}
	for p := range pkgs {
		imp.Packages = append(imp.Packages, p)
	}
	sort.Strings(imp.Packages)
	sort.Strings(imp.Entries)
	return imp, nil
}

// RenderImpactText draws each target with its callers fanning out above it.
func RenderImpactText(imp *Impact) string { return RenderImpactStyled(imp, style.Plain) }

func RenderImpactStyled(imp *Impact, st style.Palette) string {
	g := imp.graph
	rev := g.Reverse()

	var sb strings.Builder
	sb.WriteString(degradedBannerStyled(g, "  ", st))
	fmt.Fprintf(&sb, "%s %s\n", st.Title("┌── Impact of"), st.Bold(fmt.Sprintf("%q", imp.Query)))

	noCallers := st.Dim("  └── no callers in this module") + "\n"
	if imp.Degraded {
		noCallers = st.Warn("  └── no callers found — but this graph is parse-only, so treat that as unproven") + "\n"
	}

	for _, t := range imp.Targets {
		node := g.Nodes[t]
		fmt.Fprintf(&sb, "\n%s  %s\n", st.Name(t), st.Dim(fmt.Sprintf("(%s:%d)", node.File, node.Line)))
		if len(rev[t]) == 0 {
			sb.WriteString(noCallers)
			continue
		}
		visited := map[string]bool{t: true}
		renderCallers(&sb, g, rev, t, "  ", visited, 1, imp.MaxDepth, st)
	}

	sb.WriteString(st.Dim(fmt.Sprintf("\n%d %s across %d %s",
		len(imp.Callers), plural(len(imp.Callers), "caller"),
		len(imp.Packages), plural(len(imp.Packages), "package"))) + "\n")
	if len(imp.Entries) > 0 {
		fmt.Fprintf(&sb, "%s %s\n", st.Dim("reaches entry points:"), st.Ok(strings.Join(imp.Entries, ", ")))
	}
	return sb.String()
}

func renderCallers(sb *strings.Builder, g *CallGraph, rev map[string][]string, key, prefix string, visited map[string]bool, depth, maxDepth int, st style.Palette) {
	callers := rev[key]
	for i, c := range callers {
		isLast := i == len(callers)-1
		branch, child := "├── ", prefix+"│   "
		if isLast {
			branch, child = "└── ", prefix+"    "
		}

		node := g.Nodes[c]
		label := st.Accent(c)
		if node.IsEntry {
			label += st.Glyph(" 🚀", " [entry]")
		}
		fmt.Fprintf(sb, "%s%s  %s\n", st.Dim(prefix+branch), label, st.Dim(fmt.Sprintf("(%s:%d)", node.File, node.Line)))

		if visited[c] {
			fmt.Fprintf(sb, "%s\n", st.Dim(child+"└── [already shown]"))
			continue
		}
		visited[c] = true
		if maxDepth > 0 && depth >= maxDepth {
			continue
		}
		renderCallers(sb, g, rev, c, child, visited, depth+1, maxDepth, st)
	}
}

// RenderImpactMermaid draws callers pointing at the targets, targets highlighted.
func RenderImpactMermaid(imp *Impact) string {
	g := imp.graph
	rev := g.Reverse()

	inScope := map[string]bool{}
	for _, t := range imp.Targets {
		inScope[t] = true
	}
	for _, c := range imp.Callers {
		inScope[c.Key] = true
	}

	keys := make([]string, 0, len(inScope))
	for k := range inScope {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("```mermaid\nflowchart BT\n")
	for _, k := range keys {
		fmt.Fprintf(&sb, "    %s[\"%s\"]\n", mermaidID(k), k)
	}
	for _, k := range keys {
		for _, caller := range rev[k] {
			if !inScope[caller] {
				continue
			}
			fmt.Fprintf(&sb, "    %s --> %s\n", mermaidID(caller), mermaidID(k))
		}
	}
	for _, t := range imp.Targets {
		fmt.Fprintf(&sb, "    style %s fill:#43B5E6,color:#fff\n", mermaidID(t))
	}
	sb.WriteString("```\n")
	return sb.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
