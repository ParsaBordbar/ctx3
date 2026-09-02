package flow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// The type-checked path in flow.go needs a tree that builds: go/packages has to
// resolve every import and type-check every body before a call can be traced to
// its callee. That is exactly the state a repo is *not* in while an agent is
// halfway through a refactor — which is when "what calls this?" matters most.
//
// analyzeParseOnly is the fallback: go/parser only, no build, no type
// information. Call resolution becomes name-based, which costs precision in one
// specific place — a method called on a value (`x.Close()`) can only be matched
// by method name, so it is linked when exactly one type in the module declares
// that method and dropped when several do. Everything else (same-package calls,
// qualified cross-package calls) resolves exactly as before.

// maxLoadNotes bounds how many compiler errors are quoted back: the caller
// needs to know the tree is broken and roughly where, not the whole build log.
const maxLoadNotes = 5

// loadNotes reports the type-check failures that force the parse-only path,
// and is empty when every package loaded cleanly.
func loadNotes(pkgs []*packages.Package) []string {
	var notes []string
	total := 0
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			total++
			if len(notes) < maxLoadNotes {
				notes = append(notes, e.Error())
			}
		}
	})
	for _, p := range pkgs {
		if p.TypesInfo == nil && len(p.Errors) == 0 {
			total++
			if len(notes) < maxLoadNotes {
				notes = append(notes, fmt.Sprintf("package %s produced no type information", p.PkgPath))
			}
		}
	}
	if total == 0 {
		return nil
	}
	notes = append([]string{fmt.Sprintf(
		"the tree does not type-check (%d error(s)); fell back to parse-only analysis, so calls are resolved by name",
		total)}, notes...)
	if total > maxLoadNotes {
		notes = append(notes, fmt.Sprintf("… and %d more", total-maxLoadNotes))
	}
	return notes
}

// parseSkipDirs are never descended into.
var parseSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "testdata": true,
}

// parsedFile is one source file's declarations and import aliases.
type parsedFile struct {
	pkgName string
	rel     string
	// imports maps the name a file refers to a package by (alias, or the
	// package's own name) to that package's name, for in-module imports only.
	imports map[string]string
	file    *ast.File
	fset    *token.FileSet
}

// declTable is the module's declarations, indexed the three ways call
// resolution needs to look them up.
type declTable struct {
	// byPkgFunc["cmd"]["Execute"] = "cmd.Execute"
	byPkgFunc map[string]map[string]string
	// byMethod["Close"] = every "pkg.Recv.Close" key that exists
	byMethod map[string][]string
	// ambiguous counts method names that more than one type declares.
	ambiguous int
}

// analyzeParseOnly builds a call graph from syntax alone.
func analyzeParseOnly(absRoot string, cfg Config, notes []string) (*CallGraph, error) {
	graph := &CallGraph{
		Nodes:    make(map[string]*FuncNode),
		MaxDepth: cfg.MaxDepth,
		Degraded: true,
		Notes:    notes,
	}

	modPath := modulePath(absRoot)
	files, err := parseTree(absRoot, modPath)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Go files found under %s", absRoot)
	}

	table := buildDeclTable(files)
	pkgSet := make(map[string]bool)

	for _, pf := range files {
		pkgSet[pf.pkgName] = true
		collectParsedFuncs(pf, table, graph)
		collectParsedClosures(pf, table, graph)
	}

	rebuildPackages(graph, pkgSet)
	collectEntries(graph)

	if table.ambiguous > 0 {
		graph.Notes = append(graph.Notes, fmt.Sprintf(
			"%d method name(s) are declared on more than one type; calls to them were dropped rather than guessed",
			table.ambiguous))
	}

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

// modulePath reads the module path out of go.mod, or "" if there is none.
func modulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(mod)
		}
	}
	return ""
}

// parseTree parses every non-test Go file under root. Files that do not parse
// are skipped: a syntax error in one file must not blank the whole graph, which
// is the entire point of this path.
func parseTree(root, modPath string) ([]*parsedFile, error) {
	// importPath -> package name, so a qualified call can be recognized as
	// in-module without consulting the build system.
	pkgByImport := map[string]string{}
	var raw []*parsedFile

	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p == root {
				return nil
			}
			if parseSkipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if perr != nil || f.Name == nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)

		if modPath != "" {
			dir := filepath.ToSlash(filepath.Dir(rel))
			imp := modPath
			if dir != "." {
				imp = path.Join(modPath, dir)
			}
			pkgByImport[imp] = f.Name.Name
		}

		raw = append(raw, &parsedFile{pkgName: f.Name.Name, rel: rel, file: f})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Import aliases can only be resolved once every package is known.
	for _, pf := range raw {
		pf.imports = inModuleImports(pf.file, pkgByImport)
	}

	sort.Slice(raw, func(i, j int) bool { return raw[i].rel < raw[j].rel })
	// The fset is per-call, so positions must be read while it is alive.
	for _, pf := range raw {
		pf.fset = fset
	}
	return raw, nil
}

// inModuleImports maps each in-module import to the local name the file uses
// for it. Imports outside the module are omitted, which is what keeps stdlib
// and dependency calls out of the graph.
func inModuleImports(f *ast.File, pkgByImport map[string]string) map[string]string {
	out := map[string]string{}
	for _, spec := range f.Imports {
		if spec.Path == nil {
			continue
		}
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		pkgName, ok := pkgByImport[importPath]
		if !ok {
			continue // not ours
		}
		local := pkgName
		if spec.Name != nil {
			if spec.Name.Name == "_" || spec.Name.Name == "." {
				continue
			}
			local = spec.Name.Name
		}
		out[local] = pkgName
	}
	return out
}

// buildDeclTable indexes every declared function and method.
func buildDeclTable(files []*parsedFile) *declTable {
	t := &declTable{
		byPkgFunc: map[string]map[string]string{},
		byMethod:  map[string][]string{},
	}
	for _, pf := range files {
		for _, decl := range pf.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				continue
			}
			display := parsedDisplayName(fn)
			key := pf.pkgName + "." + display

			if t.byPkgFunc[pf.pkgName] == nil {
				t.byPkgFunc[pf.pkgName] = map[string]string{}
			}
			if fn.Recv == nil {
				t.byPkgFunc[pf.pkgName][fn.Name.Name] = key
			} else {
				t.byMethod[fn.Name.Name] = append(t.byMethod[fn.Name.Name], key)
			}
		}
	}
	for name, keys := range t.byMethod {
		t.byMethod[name] = unique(keys)
		if len(t.byMethod[name]) > 1 {
			t.ambiguous++
		}
	}
	return t
}

// collectParsedFuncs turns each declaration in a file into a node.
func collectParsedFuncs(pf *parsedFile, table *declTable, graph *CallGraph) {
	for _, decl := range pf.file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		display := parsedDisplayName(fn)
		key := pf.pkgName + "." + display

		node := &FuncNode{
			Package:  pf.pkgName,
			Name:     display,
			File:     pf.rel,
			Line:     pf.fset.Position(fn.Pos()).Line,
			IsEntry:  fn.Recv == nil && fn.Name.Name == "main" && pf.pkgName == "main",
			IsExport: fn.Name.IsExported(),
		}
		if fn.Body != nil {
			node.Calls = unique(parsedCalls(fn.Body, pf, table, key))
		}
		graph.Nodes[key] = node
	}
}

// collectParsedClosures mirrors collectVarClosures for the parse-only path:
// package-level vars holding func literals are entry points, since a framework
// calls them with no static caller.
func collectParsedClosures(pf *parsedFile, table *declTable, graph *CallGraph) {
	for _, decl := range pf.file.Decls {
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
				var calls []string
				ast.Inspect(vs.Values[i], func(n ast.Node) bool {
					lit, ok := n.(*ast.FuncLit)
					if !ok {
						return true
					}
					calls = append(calls, parsedCalls(lit.Body, pf, table, "")...)
					return false
				})
				if len(calls) == 0 {
					continue
				}
				key := pf.pkgName + "." + name.Name
				if _, exists := graph.Nodes[key]; exists {
					continue
				}
				graph.Nodes[key] = &FuncNode{
					Package:  pf.pkgName,
					Name:     name.Name,
					File:     pf.rel,
					Line:     pf.fset.Position(name.Pos()).Line,
					IsEntry:  true,
					IsExport: name.IsExported(),
					Calls:    unique(calls),
				}
			}
		}
	}
}

// parsedCalls resolves every call inside body to an in-module node key,
// skipping self-recursion.
func parsedCalls(body ast.Node, pf *parsedFile, table *declTable, self string) []string {
	var calls []string
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if key, ok := resolveParsedCallee(call, pf, table); ok && key != self {
			calls = append(calls, key)
		}
		return true
	})
	return calls
}

// resolveParsedCallee maps a call expression to a node key using names alone.
func resolveParsedCallee(call *ast.CallExpr, pf *parsedFile, table *declTable) (string, bool) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// A bare call is same-package, or a builtin/local variable we drop.
		if key, ok := table.byPkgFunc[pf.pkgName][fun.Name]; ok {
			return key, true
		}
		return "", false

	case *ast.SelectorExpr:
		x, ok := fun.X.(*ast.Ident)
		if ok {
			// `pkg.Func` where pkg is an in-module import resolves exactly.
			if pkgName, isImport := pf.imports[x.Name]; isImport {
				if key, found := table.byPkgFunc[pkgName][fun.Sel.Name]; found {
					return key, true
				}
				return "", false
			}
		}
		// Otherwise it is a method on a value. Without types the receiver is
		// unknown, so this is only safe when exactly one type in the module
		// declares the method; anything else would invent an edge.
		if keys := table.byMethod[fun.Sel.Name]; len(keys) == 1 {
			return keys[0], true
		}
		return "", false
	}
	return "", false
}

// parsedDisplayName is "Func" for a function, "Recv.Method" for a method.
func parsedDisplayName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return recvTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// recvTypeName strips pointers and generic parameters off a receiver type.
func recvTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr: // Recv[T]
		return recvTypeName(t.X)
	case *ast.IndexListExpr: // Recv[T, U]
		return recvTypeName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}
