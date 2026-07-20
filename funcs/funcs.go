// Package funcs extracts function and method signatures from Go source.
package funcs

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Param is one parameter or result in a signature.
type Param struct {
	Name string `json:"name,omitempty" toon:"name,omitempty"`
	Type string `json:"type"           toon:"type"`
}

// Func is a single declared function or method.
type Func struct {
	Name     string  `json:"name"                toon:"name"`
	Recv     string  `json:"recv,omitempty"      toon:"recv,omitempty"`
	TypeArgs string  `json:"typeArgs,omitempty"  toon:"typeArgs,omitempty"`
	Params   []Param `json:"params,omitempty"    toon:"params,omitempty"`
	Results  []Param `json:"results,omitempty"   toon:"results,omitempty"`
	Doc      string  `json:"doc,omitempty"       toon:"doc,omitempty"`
	File     string  `json:"file"                toon:"file"`
	Package  string  `json:"package"             toon:"package"`
	Line     int     `json:"line"                toon:"line"`
	Exported bool    `json:"exported"            toon:"exported"`
	Test     bool    `json:"test,omitempty"      toon:"test,omitempty"`
}

// Signature renders the declaration the way it appears in source, e.g. `func (g *Graph) Skill(name string) (skillwriter.Skill, error)`.
func (f Func) Signature() string {
	var b strings.Builder
	b.WriteString("func ")
	if f.Recv != "" {
		b.WriteString("(" + f.Recv + ") ")
	}
	b.WriteString(f.Name)
	b.WriteString(f.TypeArgs)
	b.WriteString("(" + joinParams(f.Params) + ")")
	switch len(f.Results) {
	case 0:
	case 1:
		if f.Results[0].Name == "" {
			b.WriteString(" " + f.Results[0].Type)
			break
		}
		b.WriteString(" (" + joinParams(f.Results) + ")")
	default:
		b.WriteString(" (" + joinParams(f.Results) + ")")
	}
	return b.String()
}

func joinParams(ps []Param) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		if p.Name == "" {
			parts[i] = p.Type
			continue
		}
		parts[i] = p.Name + " " + p.Type
	}
	return strings.Join(parts, ", ")
}

// Config controls a scan.
type Config struct {
	// Path is a .go file or a directory.
	Path string
	// Recursive walks subdirectories when Path is a directory.
	Recursive bool
	// ExportedOnly drops unexported declarations.
	ExportedOnly bool
	// IncludeTests keeps _test.go files (dropped by default).
	IncludeTests bool
	// Match, when non-nil, keeps only functions whose name matches.
	Match *regexp.Regexp
	// SortByName sorts globally by name instead of file then source order.
	SortByName bool
}

// Result is a completed scan.
type Result struct {
	Root  string `json:"root"  toon:"root"`
	Funcs []Func `json:"funcs" toon:"funcs"`
}

// skipDirs are never descended into.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "testdata": true,
}

// Scan collects signatures from cfg.Path.
func Scan(cfg Config) (*Result, error) {
	root := cfg.Path
	if root == "" {
		root = "."
	}

	files, err := collectFiles(root, cfg.Recursive)
	if err != nil {
		return nil, err
	}

	res := &Result{Root: root}
	fset := token.NewFileSet()
	for _, path := range files {
		fns, err := parseFile(fset, path, cfg)
		if err != nil {
			// A file that doesn't parse shouldn't sink the whole scan.
			continue
		}
		res.Funcs = append(res.Funcs, fns...)
	}

	if cfg.SortByName {
		sort.SliceStable(res.Funcs, func(i, j int) bool {
			a, b := res.Funcs[i], res.Funcs[j]
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.File < b.File
		})
	}
	return res, nil
}

// collectFiles returns the .go files to parse, in stable order.
func collectFiles(root string, recursive bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	var out []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// parseFile extracts the signatures of one file.
func parseFile(fset *token.FileSet, path string, cfg Config) ([]Func, error) {
	isTest := strings.HasSuffix(path, "_test.go")
	if isTest && !cfg.IncludeTests {
		return nil, nil
	}

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	// Keep the path as walked (root-prefixed) so terminal output stays clickable as file:line from the invocation directory.
	rel := filepath.ToSlash(path)

	var out []Func
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		name := fd.Name.Name
		exported := fd.Name.IsExported()
		if cfg.ExportedOnly && !exported {
			continue
		}
		if cfg.Match != nil && !cfg.Match.MatchString(name) {
			continue
		}

		out = append(out, Func{
			Name:     name,
			Recv:     recvString(fset, fd),
			TypeArgs: typeParamString(fset, fd),
			Params:   fields(fset, fd.Type.Params),
			Results:  fields(fset, fd.Type.Results),
			Doc:      docLine(fd.Doc),
			File:     rel,
			Package:  file.Name.Name,
			Line:     fset.Position(fd.Pos()).Line,
			Exported: exported,
			Test:     isTest,
		})
	}
	return out, nil
}

// recvString renders a method receiver, e.g. `g *Graph`. Empty for functions.
func recvString(fset *token.FileSet, fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	return joinParams(fields(fset, fd.Recv))
}

// typeParamString renders generic type parameters including brackets, e.g. `[T any]`.
func typeParamString(fset *token.FileSet, fd *ast.FuncDecl) string {
	if fd.Type.TypeParams == nil || len(fd.Type.TypeParams.List) == 0 {
		return ""
	}
	return "[" + joinParams(fields(fset, fd.Type.TypeParams)) + "]"
}

// fields flattens an ast.FieldList, expanding grouped names (`a, b int`) into one Param each so callers can count arity accurately.
func fields(fset *token.FileSet, fl *ast.FieldList) []Param {
	if fl == nil {
		return nil
	}
	var out []Param
	for _, f := range fl.List {
		typ := exprString(fset, f.Type)
		if len(f.Names) == 0 {
			out = append(out, Param{Type: typ})
			continue
		}
		for _, n := range f.Names {
			out = append(out, Param{Name: n.Name, Type: typ})
		}
	}
	return out
}

// exprString prints a type expression back to source form.
func exprString(fset *token.FileSet, e ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, e); err != nil {
		return "?"
	}
	return buf.String()
}

// docLine is the first sentence-ish line of a doc comment, for one-line output.
func docLine(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := strings.TrimSpace(cg.Text())
	if text == "" {
		return ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}
