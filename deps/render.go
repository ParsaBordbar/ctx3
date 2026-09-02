package deps

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"sort"
	"strings"
)

// RenderText produces a human-readable dependency chain, internal edges first,
// then a circular-import warning block if any cycles exist.
func RenderText(g *Graph) string { return RenderStyled(g, style.Plain) }

func RenderStyled(g *Graph, st style.Palette) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", st.Title("┌── Dependency Chain:"), st.Bold(g.Module))

	for i, imp := range g.Order {
		p := g.Packages[imp]
		last := i == len(g.Order)-1
		branch := "├──"
		child := "│  "
		if last {
			branch = "└──"
			child = "   "
		}
		fmt.Fprintf(&sb, "%s %s\n", st.Dim(branch), st.Name(g.ShortPath(imp)))
		for j, dep := range p.Imports {
			dLast := j == len(p.Imports)-1 && len(p.External) == 0
			b := "├─▶"
			if dLast {
				b = "└─▶"
			}
			fmt.Fprintf(&sb, "%s %s\n", st.Dim(child+" "+b), st.Accent(g.ShortPath(dep)))
		}
		if n := len(p.External); n > 0 {
			fmt.Fprintf(&sb, "%s\n", st.Dim(fmt.Sprintf("%s └── (%d external)", child, n)))
		}
	}

	if len(g.Cycles) > 0 {
		sb.WriteString("\n" + st.Err(st.Glyph("⚠", "!")+"  Circular imports detected:") + "\n")
		for _, cyc := range g.Cycles {
			short := make([]string, len(cyc))
			for i, c := range cyc {
				short[i] = g.ShortPath(c)
			}
			fmt.Fprintf(&sb, "   %s\n", st.Err(strings.Join(short, " → ")+" → "+short[0]))
		}
	} else {
		sb.WriteString("\n" + st.Ok(st.Glyph("✓", "+")+" No circular imports.") + "\n")
	}

	sb.WriteString(st.Dim(fmt.Sprintf("\n%d internal packages", len(g.Order))) + "\n")
	return sb.String()
}

// RenderMermaid produces a Mermaid dependency graph of internal packages.
// Packages in a cycle are highlighted.
func RenderMermaid(g *Graph) string {
	var sb strings.Builder
	sb.WriteString("```mermaid\nflowchart LR\n")

	inCycle := make(map[string]bool)
	for _, cyc := range g.Cycles {
		for _, c := range cyc {
			inCycle[c] = true
		}
	}

	id := func(imp string) string {
		r := strings.NewReplacer(".", "_", "/", "_", "-", "_")
		return "p_" + r.Replace(g.ShortPath(imp))
	}

	for _, imp := range g.Order {
		fmt.Fprintf(&sb, "    %s[\"%s\"]\n", id(imp), g.ShortPath(imp))
	}
	sb.WriteString("\n")

	type edge struct{ from, to string }
	seen := make(map[edge]bool)
	for _, imp := range g.Order {
		p := g.Packages[imp]
		for _, dep := range p.Imports {
			e := edge{id(imp), id(dep)}
			if seen[e] {
				continue
			}
			seen[e] = true
			fmt.Fprintf(&sb, "    %s --> %s\n", e.from, e.to)
		}
	}

	if len(inCycle) > 0 {
		sb.WriteString("\n    classDef cycle fill:#FAECE7,stroke:#993C1D,color:#712B13\n")
		cyc := make([]string, 0, len(inCycle))
		for imp := range inCycle {
			cyc = append(cyc, id(imp))
		}
		sort.Strings(cyc)
		for _, c := range cyc {
			fmt.Fprintf(&sb, "    class %s cycle\n", c)
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}
