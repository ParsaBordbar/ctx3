// Package deps builds the internal dependency chain of a Go module: which
// packages import which, split from external dependencies, plus circular-import
// detection. It is the fact-source behind the `deps` command and (later) a
// progressive-disclosure skill reference.
package deps

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Config controls analysis.
type Config struct {
	RootDir string
}

// Package is one internal package and its dependency edges.
type Package struct {
	ImportPath string   `json:"import_path" toon:"import_path"`
	Dir        string   `json:"dir" toon:"dir"`
	Name       string   `json:"name" toon:"name"`
	Imports    []string `json:"imports" toon:"imports"`     // internal import paths
	External   []string `json:"external" toon:"external"`   // third-party/std import paths
}

// Graph is the whole internal dependency chain.
type Graph struct {
	Module   string              `json:"module" toon:"module"`
	Packages map[string]*Package `json:"-" toon:"-"`       // lookup index (not serialized)
	List     []*Package          `json:"packages" toon:"packages"` // serialized, sorted by import path
	Order    []string            `json:"order" toon:"order"`       // import paths, sorted
	Cycles   [][]string          `json:"cycles" toon:"cycles"`
}

// Analyze walks RootDir, groups .go files by package directory, and builds the
// internal import graph plus cycle list. Test files and vendored/hidden dirs
// are skipped.
func Analyze(cfg Config) (*Graph, error) {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}
	module, err := modulePath(cfg.RootDir)
	if err != nil {
		return nil, err
	}

	g := &Graph{Module: module, Packages: make(map[string]*Package)}

	err = filepath.WalkDir(cfg.RootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if path != cfg.RootDir && (strings.HasPrefix(base, ".") ||
				base == "vendor" || base == "node_modules" || base == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return collectFile(path, cfg.RootDir, module, g)
	})
	if err != nil {
		return nil, err
	}

	// Finalize each package: dedup + resolve internal vs external, sort.
	for _, p := range g.Packages {
		p.Imports = sortedUnique(p.Imports)
		p.External = sortedUnique(p.External)
		g.Order = append(g.Order, p.ImportPath)
	}
	sort.Strings(g.Order)

	// Build the serialized, ordered package list from the index.
	for _, imp := range g.Order {
		g.List = append(g.List, g.Packages[imp])
	}

	g.Cycles = findCycles(g)
	return g, nil
}

func collectFile(path, root, module string, g *Graph) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return nil // skip unparseable
	}

	dir := filepath.Dir(path)
	rel, _ := filepath.Rel(root, dir)
	rel = filepath.ToSlash(rel)

	impPath := module
	if rel != "." && rel != "" {
		impPath = module + "/" + rel
	}

	p := g.Packages[impPath]
	if p == nil {
		p = &Package{ImportPath: impPath, Dir: rel, Name: f.Name.Name}
		g.Packages[impPath] = p
	}

	for _, spec := range f.Imports {
		imp, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if imp == module || strings.HasPrefix(imp, module+"/") {
			p.Imports = append(p.Imports, imp)
		} else {
			p.External = append(p.External, imp)
		}
	}
	return nil
}

// findCycles returns import cycles among internal packages via DFS.
func findCycles(g *Graph) [][]string {
	const (
		white = 0 // unvisited
		gray  = 1 // on stack
		black = 2 // done
	)
	color := make(map[string]int, len(g.Packages))
	var stack []string
	var cycles [][]string
	seen := make(map[string]bool)

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		stack = append(stack, node)

		if p := g.Packages[node]; p != nil {
			for _, dep := range p.Imports {
				switch color[dep] {
				case white:
					dfs(dep)
				case gray:
					// Found a back-edge: extract the cycle from the stack.
					cyc := extractCycle(stack, dep)
					if key := cycleKey(cyc); !seen[key] {
						seen[key] = true
						cycles = append(cycles, cyc)
					}
				}
			}
		}

		stack = stack[:len(stack)-1]
		color[node] = black
	}

	for _, node := range g.Order {
		if color[node] == white {
			dfs(node)
		}
	}
	sort.Slice(cycles, func(i, j int) bool { return cycleKey(cycles[i]) < cycleKey(cycles[j]) })
	return cycles
}

func extractCycle(stack []string, start string) []string {
	for i, n := range stack {
		if n == start {
			cyc := append([]string(nil), stack[i:]...)
			return cyc
		}
	}
	return append([]string(nil), stack...)
}

// cycleKey canonicalizes a cycle (rotation-invariant) for dedup.
func cycleKey(cyc []string) string {
	if len(cyc) == 0 {
		return ""
	}
	min := 0
	for i := 1; i < len(cyc); i++ {
		if cyc[i] < cyc[min] {
			min = i
		}
	}
	rotated := append(append([]string(nil), cyc[min:]...), cyc[:min]...)
	return strings.Join(rotated, "->")
}

func sortedUnique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := ss[:0]
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// modulePath reads the module path from go.mod at root.
func modulePath(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("no go.mod at %s: dependency chain currently supports Go modules", root)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("go.mod at %s has no module directive", root)
}

// ShortPath trims the module prefix so output stays readable.
func (g *Graph) ShortPath(imp string) string {
	if imp == g.Module {
		return "."
	}
	return strings.TrimPrefix(imp, g.Module+"/")
}
