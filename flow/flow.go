package flow

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Config struct {
	RootDir   string
	EntryOnly bool // keep only functions reachable from entry points
	MaxDepth  int  // 0 = unlimited; caps render/traversal depth
}

// FuncNode represents a function and the functions it calls.
type FuncNode struct {
	Package  string   `json:"package" toon:"package"`
	Name     string   `json:"name" toon:"name"`
	File     string   `json:"file" toon:"file"`
	Line     int      `json:"line" toon:"line"`
	Calls    []string `json:"calls" toon:"calls"`
	IsEntry  bool     `json:"isEntry" toon:"is_entry"`
	IsExport bool     `json:"isExport" toon:"is_export"`
}

// CallGraph holds all discovered functions and their call edges.
type CallGraph struct {
	// Nodes keyed by "package.FuncName" (methods: "package.Recv.Method")
	Nodes map[string]*FuncNode `json:"nodes" toon:"nodes"`
	// Entry points (nodes in entry files)
	Entries []string `json:"entries" toon:"entries"`
	// Package list
	Packages []string `json:"packages" toon:"packages"`
	// MaxDepth caps how deep renderers traverse (0 = unlimited).
	MaxDepth int `json:"maxDepth" toon:"max_depth"`
	// Degraded reports that the graph was built from syntax alone because the
	// tree does not type-check. Edges are then resolved by name, so a method
	// called on a value is only linked when one type in the module declares it.
	Degraded bool `json:"degraded" toon:"degraded"`
	// Notes explain why the analysis degraded, for the caller to surface.
	Notes []string `json:"notes,omitempty" toon:"notes,omitempty"`
}

// packagesLoadMode carries syntax + full type info + deps for call resolution.
const packagesLoadMode = packages.NeedName | packages.NeedFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
	packages.NeedImports | packages.NeedDeps

// AnalyzeFlow type-checks the packages under rootDir and builds a call graph.
func AnalyzeFlow(cfg Config) (*CallGraph, error) {
	graph := &CallGraph{
		Nodes:    make(map[string]*FuncNode),
		MaxDepth: cfg.MaxDepth,
	}

	// go/packages reports absolute paths; relative output needs an absolute root.
	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolving root %q: %w", cfg.RootDir, err)
	}

	pkgs, err := packages.Load(&packages.Config{
		Mode:  packagesLoadMode,
		Dir:   cfg.RootDir,
		Tests: false,
	}, "./...")
	if err != nil {
		// The tree could not be loaded at all — syntax alone is all that is left.
		return analyzeParseOnly(absRoot, cfg, []string{
			fmt.Sprintf("go/packages could not load the tree (%v); fell back to parse-only analysis", err),
		})
	}

	// Type information is all-or-nothing for call resolution: a package that
	// failed to type-check yields no edges at all, which reads as "this code
	// calls nothing" rather than "this could not be analyzed". Silently mixing
	// the two would be worse than degrading the whole graph, so any error sends
	// the entire analysis down the parse-only path.
	if notes := loadNotes(pkgs); len(notes) > 0 {
		return analyzeParseOnly(absRoot, cfg, notes)
	}

	// In-module package paths — edges to anything else (stdlib, deps) are dropped.
	ours := make(map[string]bool, len(pkgs))
	for _, p := range pkgs {
		ours[p.PkgPath] = true
	}

	pkgSet := make(map[string]bool)
	for _, p := range pkgs {
		if p.TypesInfo == nil {
			continue
		}
		collectPackage(p, absRoot, ours, graph, pkgSet)
		collectVarClosures(p, absRoot, ours, graph)
	}

	rebuildPackages(graph, pkgSet)
	collectEntries(graph)

	// A clean load that produced nothing means the type-checked path found no
	// declarations it could key; syntax will at least find the functions.
	if len(graph.Nodes) == 0 {
		return analyzeParseOnly(absRoot, cfg, []string{
			"type-checking produced no call graph; fell back to parse-only analysis",
		})
	}

	// --entry-only: drop nodes unreachable from an entry point.
	if cfg.EntryOnly && len(graph.Entries) > 0 {
		keep := reachableFrom(graph, graph.Entries, 0)
		for k := range graph.Nodes {
			if !keep[k] {
				delete(graph.Nodes, k)
			}
		}
		pruned := make(map[string]bool)
		for _, n := range graph.Nodes {
			pruned[n.Package] = true
		}
		rebuildPackages(graph, pruned)
	}

	return graph, nil
}

func collectPackage(p *packages.Package, rootDir string, ours map[string]bool, graph *CallGraph, pkgSet map[string]bool) {
	pkgSet[p.Name] = true

	for _, file := range p.Syntax {
		tf := p.Fset.File(file.Pos())
		if tf == nil {
			continue
		}
		filename := tf.Name()
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		rel, _ := filepath.Rel(rootDir, filename)
		rel = filepath.ToSlash(rel)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			obj, _ := p.TypesInfo.Defs[fn.Name].(*types.Func)
			if obj == nil || obj.Pkg() == nil {
				continue
			}

			key := funcKey(obj)
			node := &FuncNode{
				Package:  obj.Pkg().Name(),
				Name:     funcDisplayName(obj),
				File:     rel,
				Line:     p.Fset.Position(fn.Pos()).Line,
				IsEntry:  isProgramEntry(obj),
				IsExport: obj.Exported(),
			}

			if fn.Body != nil {
				// Inspect descends into nested closures, crediting their calls here.
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if call, ok := n.(*ast.CallExpr); ok {
						if ck, ok := resolveCallee(p.TypesInfo, ours, call); ok && ck != key {
							node.Calls = append(node.Calls, ck)
						}
					}
					return true
				})
			}

			node.Calls = unique(node.Calls)
			graph.Nodes[key] = node
		}
	}
}

// collectVarClosures makes each package-level var holding func literals (e.g.
// cobra RunE handlers, which have no static caller) an entry node whose edges
// are the functions those closures call.
func collectVarClosures(p *packages.Package, rootDir string, ours map[string]bool, graph *CallGraph) {
	for _, file := range p.Syntax {
		tf := p.Fset.File(file.Pos())
		if tf == nil || strings.HasSuffix(tf.Name(), "_test.go") {
			continue
		}
		rel, _ := filepath.Rel(rootDir, tf.Name())
		rel = filepath.ToSlash(rel)

		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					calls := closureCalls(p.TypesInfo, ours, vs.Values[i])
					if len(calls) == 0 {
						continue
					}
					key := p.Name + "." + name.Name
					if _, exists := graph.Nodes[key]; exists {
						continue
					}
					graph.Nodes[key] = &FuncNode{
						Package:  p.Name,
						Name:     name.Name,
						File:     rel,
						Line:     p.Fset.Position(name.Pos()).Line,
						IsEntry:  true,
						IsExport: name.IsExported(),
						Calls:    unique(calls),
					}
				}
			}
		}
	}
}

// closureCalls returns in-module calls made inside func literals within expr.
func closureCalls(info *types.Info, ours map[string]bool, expr ast.Expr) []string {
	var calls []string
	ast.Inspect(expr, func(n ast.Node) bool {
		lit, ok := n.(*ast.FuncLit)
		if !ok {
			return true
		}
		ast.Inspect(lit.Body, func(m ast.Node) bool {
			if call, ok := m.(*ast.CallExpr); ok {
				if ck, ok := resolveCallee(info, ours, call); ok {
					calls = append(calls, ck)
				}
			}
			return true
		})
		return false // body already walked
	})
	return calls
}

// resolveCallee maps a call to an in-module node key, or false if not ours.
func resolveCallee(info *types.Info, ours map[string]bool, call *ast.CallExpr) (string, bool) {
	var id *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	default:
		return "", false
	}
	callee, _ := info.Uses[id].(*types.Func)
	if callee == nil || callee.Pkg() == nil {
		return "", false
	}
	if !ours[callee.Pkg().Path()] {
		return "", false
	}
	return funcKey(callee), true
}

func rebuildPackages(graph *CallGraph, pkgSet map[string]bool) {
	graph.Packages = graph.Packages[:0]
	for pkg := range pkgSet {
		graph.Packages = append(graph.Packages, pkg)
	}
	sort.Strings(graph.Packages)
}

func collectEntries(graph *CallGraph) {
	graph.Entries = graph.Entries[:0]
	for key, node := range graph.Nodes {
		if node.IsEntry {
			graph.Entries = append(graph.Entries, key)
		}
	}
	sort.Strings(graph.Entries)
}

// isProgramEntry reports whether obj is package main's `func main`.
func isProgramEntry(obj *types.Func) bool {
	if obj.Name() != "main" || obj.Pkg() == nil || obj.Pkg().Name() != "main" {
		return false
	}
	sig, ok := obj.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}

// funcKey is the canonical node key: "pkg.Func" or "pkg.Recv.Method".
func funcKey(obj *types.Func) string {
	return obj.Pkg().Name() + "." + funcDisplayName(obj)
}

// funcDisplayName is "Func" for a function, "Recv.Method" for a method.
func funcDisplayName(obj *types.Func) string {
	name := obj.Name()
	if sig, ok := obj.Type().(*types.Signature); ok && sig.Recv() != nil {
		rt := sig.Recv().Type()
		if ptr, ok := rt.(*types.Pointer); ok {
			rt = ptr.Elem()
		}
		if named, ok := rt.(*types.Named); ok {
			name = named.Obj().Name() + "." + name
		}
	}
	return name
}

// reachableFrom returns node keys reachable from roots (BFS). maxDepth 0 = unlimited.
func reachableFrom(g *CallGraph, roots []string, maxDepth int) map[string]bool {
	keep := make(map[string]bool)
	type item struct {
		key   string
		depth int
	}
	var queue []item
	for _, r := range roots {
		if _, ok := g.Nodes[r]; ok && !keep[r] {
			keep[r] = true
			queue = append(queue, item{r, 0})
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		node := g.Nodes[cur.key]
		if node == nil {
			continue
		}
		for _, c := range node.Calls {
			if _, ok := g.Nodes[c]; !ok {
				continue
			}
			if !keep[c] {
				keep[c] = true
				queue = append(queue, item{c, cur.depth + 1})
			}
		}
	}
	return keep
}

// rootKeys returns entry points, or all exported functions if none were found.
func rootKeys(g *CallGraph) []string {
	if len(g.Entries) > 0 {
		return g.Entries
	}
	var roots []string
	for key, node := range g.Nodes {
		if node.IsExport {
			roots = append(roots, key)
		}
	}
	sort.Strings(roots)
	return roots
}

func unique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// degradedBanner warns that the graph came from the parse-only path. A caller
// that cannot tell a name-resolved edge from a type-checked one would read a
// missing method call as "nothing calls this", so the caveat leads the output
// rather than trailing it.
func degradedBanner(g *CallGraph, indent string) string {
	return degradedBannerStyled(g, indent, style.Plain)
}

func degradedBannerStyled(g *CallGraph, indent string, st style.Palette) string {
	if g == nil || !g.Degraded {
		return ""
	}
	if st.Enabled() {
		return st.Warn(strings.TrimRight(degradedBanner(g, indent), "\n")) + "\n\n"
	}
	var sb strings.Builder
	sb.WriteString(indent + "⚠ parse-only analysis — this tree does not type-check.\n")
	sb.WriteString(indent + "  Calls are resolved by name: a method called on a value is linked\n")
	sb.WriteString(indent + "  only when one type in the module declares it, so some edges are missing.\n")
	for _, n := range g.Notes {
		fmt.Fprintf(&sb, "%s  · %s\n", indent, n)
	}
	sb.WriteString("\n")
	return sb.String()
}

// ─── Text renderer ───────────────────────────────────────────────────────────

// RenderText produces a human-readable call-tree. Starts from entry points
// (or all exported functions if no entries are found). Honors g.MaxDepth.
func RenderText(g *CallGraph) string { return RenderStyled(g, style.Plain) }

func RenderStyled(g *CallGraph, st style.Palette) string {
	var sb strings.Builder
	sb.WriteString(degradedBannerStyled(g, "  ", st))
	sb.WriteString(st.Title("┌── Code Flow") + "\n")

	roots := rootKeys(g)

	visited := make(map[string]bool)
	for _, r := range roots {
		renderTextNode(&sb, g, r, "├── ", "│   ", visited, 0, st)
	}

	sb.WriteString(st.Dim(fmt.Sprintf("\n%d functions across %d packages", len(g.Nodes), len(g.Packages))) + "\n")
	return sb.String()
}

func renderTextNode(sb *strings.Builder, g *CallGraph, key, prefix, childPrefix string, visited map[string]bool, depth int, st style.Palette) {
	node, ok := g.Nodes[key]
	if !ok {
		return
	}

	label := st.Accent(node.Package + "." + node.Name)
	if node.IsEntry {
		label += " 🚀"
	}
	fmt.Fprintf(sb, "%s%s  %s\n", st.Dim(prefix), label, st.Dim(fmt.Sprintf("(%s:%d)", node.File, node.Line)))

	if visited[key] {
		fmt.Fprintf(sb, "%s\n", st.Dim(childPrefix+"  └── [already shown]"))
		return
	}
	visited[key] = true

	if g.MaxDepth > 0 && depth+1 > g.MaxDepth {
		return
	}

	for i, callee := range node.Calls {
		if _, ok := g.Nodes[callee]; !ok {
			continue
		}
		isLast := i == len(node.Calls)-1
		p, cp := childPrefix+"├── ", childPrefix+"│   "
		if isLast {
			p, cp = childPrefix+"└── ", childPrefix+"    "
		}
		renderTextNode(sb, g, callee, p, cp, visited, depth+1, st)
	}
}

// ─── Mermaid renderer ────────────────────────────────────────────────────────

func RenderMermaid(g *CallGraph) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\nflowchart LR\n")

	// With a depth cap, emit only nodes reachable within that depth.
	var keep map[string]bool
	if g.MaxDepth > 0 {
		keep = reachableFrom(g, rootKeys(g), g.MaxDepth)
	}
	included := func(key string) bool { return keep == nil || keep[key] }

	// Assign a color class per package
	pkgClass := make(map[string]string, len(g.Packages))
	classes := []string{"pkgA", "pkgB", "pkgC", "pkgD", "pkgE"}
	for i, pkg := range g.Packages {
		pkgClass[pkg] = classes[i%len(classes)]
	}

	// Emit nodes — sanitize key to valid Mermaid ID
	keys := make([]string, 0, len(g.Nodes))
	for k := range g.Nodes {
		if included(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	for _, key := range keys {
		node := g.Nodes[key]
		id := mermaidID(key)
		shape := fmt.Sprintf("%s[\"%s.%s\"]", id, node.Package, node.Name)
		if node.IsEntry {
			shape = fmt.Sprintf("%s([\"%s.%s\"])", id, node.Package, node.Name)
		}
		fmt.Fprintf(&sb, "    %s\n", shape)
	}

	sb.WriteString("\n")

	// Emit edges (deduplicated)
	type edge struct{ from, to string }
	seen := make(map[edge]bool)
	for _, key := range keys {
		node := g.Nodes[key]
		fromID := mermaidID(key)
		for _, callee := range node.Calls {
			if _, exists := g.Nodes[callee]; !exists {
				continue
			}
			if !included(callee) {
				continue
			}
			toID := mermaidID(callee)
			e := edge{fromID, toID}
			if seen[e] {
				continue
			}
			seen[e] = true
			fmt.Fprintf(&sb, "    %s --> %s\n", fromID, toID)
		}
	}

	sb.WriteString("\n")

	// Emit class definitions and assignments
	sb.WriteString("    classDef pkgA fill:#EEEDFE,stroke:#534AB7,color:#3C3489\n")
	sb.WriteString("    classDef pkgB fill:#E1F5EE,stroke:#0F6E56,color:#085041\n")
	sb.WriteString("    classDef pkgC fill:#FAECE7,stroke:#993C1D,color:#712B13\n")
	sb.WriteString("    classDef pkgD fill:#E6F1FB,stroke:#185FA5,color:#0C447C\n")
	sb.WriteString("    classDef pkgE fill:#FAEEDA,stroke:#854F0B,color:#633806\n")
	sb.WriteString("    classDef entrypoint fill:#EAF3DE,stroke:#3B6D11,color:#27500A\n\n")

	for _, key := range keys {
		node := g.Nodes[key]
		id := mermaidID(key)
		cls := pkgClass[node.Package]
		if node.IsEntry {
			cls = "entrypoint"
		}
		fmt.Fprintf(&sb, "    class %s %s\n", id, cls)
	}

	sb.WriteString("```\n")
	return sb.String()
}

// mermaidID converts "pkg.Func" into a valid Mermaid node identifier.
func mermaidID(key string) string {
	r := strings.NewReplacer(".", "_", "-", "_", "/", "_", " ", "_")
	return r.Replace(key)
}
