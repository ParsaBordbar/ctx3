package flow

import (
	"fmt"
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
	imp := &Impact{Query: query, Targets: targets, MaxDepth: maxDepth, graph: g}

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
func RenderImpactText(imp *Impact) string {
	g := imp.graph
	rev := g.Reverse()

	var sb strings.Builder
	fmt.Fprintf(&sb, "┌── Impact of %q\n", imp.Query)

	for _, t := range imp.Targets {
		node := g.Nodes[t]
		fmt.Fprintf(&sb, "\n%s  (%s:%d)\n", t, node.File, node.Line)
		if len(rev[t]) == 0 {
			sb.WriteString("  └── no callers in this module\n")
			continue
		}
		visited := map[string]bool{t: true}
		renderCallers(&sb, g, rev, t, "  ", visited, 1, imp.MaxDepth)
	}

	fmt.Fprintf(&sb, "\n%d %s across %d %s\n",
		len(imp.Callers), plural(len(imp.Callers), "caller"),
		len(imp.Packages), plural(len(imp.Packages), "package"))
	if len(imp.Entries) > 0 {
		fmt.Fprintf(&sb, "reaches entry points: %s\n", strings.Join(imp.Entries, ", "))
	}
	return sb.String()
}

func renderCallers(sb *strings.Builder, g *CallGraph, rev map[string][]string, key, prefix string, visited map[string]bool, depth, maxDepth int) {
	callers := rev[key]
	for i, c := range callers {
		isLast := i == len(callers)-1
		branch, child := "├── ", prefix+"│   "
		if isLast {
			branch, child = "└── ", prefix+"    "
		}

		node := g.Nodes[c]
		label := c
		if node.IsEntry {
			label += " 🚀"
		}
		fmt.Fprintf(sb, "%s%s%s  (%s:%d)\n", prefix, branch, label, node.File, node.Line)

		if visited[c] {
			fmt.Fprintf(sb, "%s└── [already shown]\n", child)
			continue
		}
		visited[c] = true
		if maxDepth > 0 && depth >= maxDepth {
			continue
		}
		renderCallers(sb, g, rev, c, child, visited, depth+1, maxDepth)
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
