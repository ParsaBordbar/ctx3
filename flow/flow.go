package flow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	RootDir   string
	EntryOnly bool // only trace from entry-point files
	MaxDepth  int  // 0 = unlimited
}

// FuncNode represents a function and the functions it calls.
type FuncNode struct {
	Package  string
	Name     string
	File     string
	Line     int
	Calls    []string
	IsEntry  bool
	IsExport bool
}

// CallGraph holds all discovered functions and their call edges.
type CallGraph struct {
	// Nodes keyed by "package.FuncName"
	Nodes map[string]*FuncNode
	// Entry points (nodes in entry files)
	Entries []string
	// Package list
	Packages []string
}

var entryFileNames = map[string]bool{
	"main.go": true, "server.go": true, "app.go": true, "index.go": true,
}

// AnalyzeFlow walks rootDir and builds a call graph from Go source files.
func AnalyzeFlow(cfg Config) (*CallGraph, error) {
	graph := &CallGraph{
		Nodes: make(map[string]*FuncNode),
	}

	pkgSet := make(map[string]bool)

	err := filepath.WalkDir(cfg.RootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if base == "vendor" || base == "node_modules" || base == ".git" ||
				base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		return parseGoFile(path, cfg.RootDir, graph, pkgSet)
	})
	if err != nil {
		return nil, err
	}

	// Build package list
	for pkg := range pkgSet {
		graph.Packages = append(graph.Packages, pkg)
	}
	sort.Strings(graph.Packages)

	// Collect entry nodes
	for key, node := range graph.Nodes {
		if node.IsEntry {
			graph.Entries = append(graph.Entries, key)
		}
	}
	sort.Strings(graph.Entries)

	return graph, nil
}

func parseGoFile(path, rootDir string, graph *CallGraph, pkgSet map[string]bool) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		// Skip unparseable files gracefully
		return nil
	}

	pkgName := f.Name.Name
	pkgSet[pkgName] = true

	rel, _ := filepath.Rel(rootDir, path)
	rel = filepath.ToSlash(rel)
	isEntry := entryFileNames[filepath.Base(path)]

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		funcName := fn.Name.Name
		key := pkgName + "." + funcName
		if fn.Recv != nil {
			// Method — include receiver type in name
			recvType := receiverTypeName(fn.Recv)
			funcName = recvType + "." + funcName
			key = pkgName + "." + funcName
		}

		node := &FuncNode{
			Package:  pkgName,
			Name:     funcName,
			File:     rel,
			Line:     fset.Position(fn.Pos()).Line,
			IsEntry:  isEntry,
			IsExport: len(funcName) > 0 && funcName[0] >= 'A' && funcName[0] <= 'Z',
		}

		// Walk the function body for call expressions
		if fn.Body != nil {
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := calleeString(call.Fun, pkgName)
				if callee != "" && callee != key {
					node.Calls = append(node.Calls, callee)
				}
				return true
			})
		}

		// Deduplicate calls
		node.Calls = unique(node.Calls)

		graph.Nodes[key] = node
	}

	return nil
}

func receiverTypeName(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	switch t := fl.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func calleeString(expr ast.Expr, currentPkg string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		// Local call — qualify with current package
		return currentPkg + "." + e.Name
	case *ast.SelectorExpr:
		// pkg.Func or recv.Method
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name + "." + e.Sel.Name
		}
	}
	return ""
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

// ─── Text renderer ───────────────────────────────────────────────────────────

// RenderText produces a human-readable call-tree. Starts from entry points
// (or all exported functions if no entries are found).
func RenderText(g *CallGraph) string {
	var sb strings.Builder
	sb.WriteString("┌── Code Flow\n")

	roots := g.Entries
	if len(roots) == 0 {
		// Fallback: exported functions sorted
		for key, node := range g.Nodes {
			if node.IsExport {
				roots = append(roots, key)
			}
		}
		sort.Strings(roots)
	}

	visited := make(map[string]bool)
	for _, r := range roots {
		renderTextNode(&sb, g, r, "├── ", "│   ", visited, 0)
	}

	// Summary
	fmt.Fprintf(&sb, "\n%d functions across %d packages\n",
		len(g.Nodes), len(g.Packages))
	return sb.String()
}

func renderTextNode(sb *strings.Builder, g *CallGraph, key, prefix, childPrefix string, visited map[string]bool, depth int) {
	node, ok := g.Nodes[key]
	if !ok {
		return
	}

	label := node.Package + "." + node.Name
	if node.IsEntry {
		label += " 🚀"
	}
	fmt.Fprintf(sb, "%s%s  (%s:%d)\n", prefix, label, node.File, node.Line)

	if visited[key] {
		fmt.Fprintf(sb, "%s  └── [already shown]\n", childPrefix)
		return
	}
	visited[key] = true

	for i, callee := range node.Calls {
		isLast := i == len(node.Calls)-1
		p, cp := childPrefix+"├── ", childPrefix+"│   "
		if isLast {
			p, cp = childPrefix+"└── ", childPrefix+"    "
		}
		renderTextNode(sb, g, callee, p, cp, visited, depth+1)
	}
}

// ─── Mermaid renderer ────────────────────────────────────────────────────────


func RenderMermaid(g *CallGraph) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\nflowchart LR\n")

	// Assign a color class per package
	pkgClass := make(map[string]string, len(g.Packages))
	classes := []string{"pkgA", "pkgB", "pkgC", "pkgD", "pkgE"}
	for i, pkg := range g.Packages {
		pkgClass[pkg] = classes[i%len(classes)]
	}

	// Emit nodes — sanitize key to valid Mermaid ID
	keys := make([]string, 0, len(g.Nodes))
	for k := range g.Nodes {
		keys = append(keys, k)
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