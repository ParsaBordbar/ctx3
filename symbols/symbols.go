// Package symbols builds a flat, parse-only index of every top-level declaration in a Go tree.
package symbols

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

	ignore "github.com/sabhiram/go-gitignore"
)

// Kind classifies a declaration.
type Kind string

const (
	KindFunc      Kind = "func"
	KindMethod    Kind = "method"
	KindStruct    Kind = "struct"
	KindInterface Kind = "interface"
	KindType      Kind = "type"
	KindConst     Kind = "const"
	KindVar       Kind = "var"
)

// kindOrder is the print order within a package.
var kindOrder = map[Kind]int{
	KindConst: 0, KindVar: 1, KindStruct: 2, KindInterface: 3,
	KindType: 4, KindFunc: 5, KindMethod: 6,
}

// Member is a struct field or an interface method.
type Member struct {
	Name string `json:"name"          toon:"name"`
	Type string `json:"type"          toon:"type"`
	Doc  string `json:"doc,omitempty" toon:"doc,omitempty"`
}

// Symbol is one top-level declaration.
type Symbol struct {
	Name      string   `json:"name"                 toon:"name"`
	Kind      Kind     `json:"kind"                 toon:"kind"`
	Recv      string   `json:"recv,omitempty"       toon:"recv,omitempty"`
	Signature string   `json:"signature"            toon:"signature"`
	Doc       string   `json:"doc,omitempty"        toon:"doc,omitempty"`
	Members   []Member `json:"members,omitempty"    toon:"members,omitempty"`
	Lang      string   `json:"lang"                 toon:"lang"`
	Package   string   `json:"package"              toon:"package"`
	Dir       string   `json:"dir"                  toon:"dir"`
	File      string   `json:"file"                 toon:"file"`
	Line      int      `json:"line"                 toon:"line"`
	Exported  bool     `json:"exported"             toon:"exported"`
}

// Index is a completed scan.
type Index struct {
	Root    string   `json:"root"    toon:"root"`
	Symbols []Symbol `json:"symbols" toon:"symbols"`
}

// Config controls a scan.
type Config struct {
	// Path is a .go file or a directory.
	Path string
	// NoRecurse stops at Path when it is a directory.
	NoRecurse bool
	// IncludeUnexported keeps lowercase declarations.
	IncludeUnexported bool
	// IncludeTests keeps _test.go files.
	IncludeTests bool
	// Members fills struct fields and interface methods.
	Members bool
	// Kinds, when non-empty, keeps only these kinds.
	Kinds []Kind
	// Match, when non-nil, keeps only symbols whose name matches.
	Match *regexp.Regexp
	// Langs, when non-empty, keeps only these languages ("go", "python", …).
	Langs []string
}

// skipDirs are never indexed. Build-output names like dist/, build/ and
// target/ are deliberately absent: they are also perfectly ordinary package
// names (ctx3 has its own target/), and silently dropping real symbols is a
// worse failure than indexing a generated copy. Generated *files* are filtered
// instead, by name and size, in lang.go.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "testdata": true,
	"__pycache__": true,
}

// Scan indexes every declaration under cfg.Path.
func Scan(cfg Config) (*Index, error) {
	root := cfg.Path
	if root == "" {
		root = "."
	}
	files, err := collectFiles(root, !cfg.NoRecurse)
	if err != nil {
		return nil, err
	}

	keep := map[Kind]bool{}
	for _, k := range cfg.Kinds {
		keep[k] = true
	}
	keepLang := map[string]bool{}
	for _, l := range cfg.Langs {
		keepLang[strings.ToLower(strings.TrimSpace(l))] = true
	}

	idx := &Index{Root: root}
	fset := token.NewFileSet()
	for _, path := range files {
		var (
			syms []Symbol
			err  error
		)
		if spec := langByExt[strings.ToLower(filepath.Ext(path))]; spec != nil {
			syms, err = scanLangFile(path, spec, cfg)
		} else {
			syms, err = parseFile(fset, path, cfg)
		}
		if err != nil {
			// A file that doesn't parse shouldn't sink the whole scan.
			continue
		}
		for _, s := range syms {
			if len(keep) > 0 && !keep[s.Kind] {
				continue
			}
			if len(keepLang) > 0 && !keepLang[s.Lang] {
				continue
			}
			idx.Symbols = append(idx.Symbols, s)
		}
	}

	sort.SliceStable(idx.Symbols, func(i, j int) bool {
		a, b := idx.Symbols[i], idx.Symbols[j]
		if a.Dir != b.Dir {
			return a.Dir < b.Dir
		}
		if kindOrder[a.Kind] != kindOrder[b.Kind] {
			return kindOrder[a.Kind] < kindOrder[b.Kind]
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Line < b.Line
	})
	return idx, nil
}

// Packages returns the directories in the index, in order.
func (idx *Index) Packages() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range idx.Symbols {
		if !seen[s.Dir] {
			seen[s.Dir] = true
			out = append(out, s.Dir)
		}
	}
	return out
}

func collectFiles(root string, recursive bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{root}, nil
	}

	// The repo already declares what is not source. Honoring .gitignore is what
	// keeps build output — dist/, build/, a compiled bundle — out of the index
	// without hard-coding directory names that are also ordinary package names.
	gitIg := loadGitignore(root)
	ignored := func(path string, isDir bool) bool {
		if gitIg == nil {
			return false
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return false
		}
		rel = filepath.ToSlash(rel)
		if isDir {
			rel += "/"
		}
		return gitIg.MatchesPath(rel)
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
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") || ignored(path, true) {
				return filepath.SkipDir
			}
			if !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if isIndexable(path) && !ignored(path, false) {
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

// loadGitignore compiles the root .gitignore, or nil when there is none.
func loadGitignore(root string) *ignore.GitIgnore {
	gi, err := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return nil
	}
	return gi
}

// isIndexable reports whether a path is a source file the scanner understands.
func isIndexable(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".go" || langByExt[ext] != nil
}

func parseFile(fset *token.FileSet, path string, cfg Config) ([]Symbol, error) {
	if strings.HasSuffix(path, "_test.go") && !cfg.IncludeTests {
		return nil, nil
	}
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}

	rel := filepath.ToSlash(path)
	dir := filepath.ToSlash(filepath.Dir(path))
	base := Symbol{Package: file.Name.Name, Lang: "go", Dir: dir, File: rel}

	var out []Symbol
	add := func(s Symbol, name *ast.Ident, pos token.Pos) {
		if cfg.Match != nil && !cfg.Match.MatchString(s.Name) {
			return
		}
		s.Exported = name.IsExported()
		if !s.Exported && !cfg.IncludeUnexported {
			return
		}
		s.Line = fset.Position(pos).Line
		out = append(out, s)
	}

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			s := base
			s.Name = d.Name.Name
			s.Kind = KindFunc
			s.Recv = recvString(fset, d.Recv)
			if s.Recv != "" {
				s.Kind = KindMethod
			}
			s.Signature = funcSignature(fset, d)
			s.Doc = docLine(d.Doc)
			add(s, d.Name, d.Pos())

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					s := base
					s.Name = sp.Name.Name
					s.Kind, s.Members = typeKind(fset, sp, cfg.Members)
					s.Signature = typeSignature(fset, sp)
					s.Doc = firstDoc(d.Doc, sp.Doc)
					add(s, sp.Name, sp.Pos())

				case *ast.ValueSpec:
					kind := KindVar
					if d.Tok == token.CONST {
						kind = KindConst
					}
					for i, name := range sp.Names {
						s := base
						s.Name = name.Name
						s.Kind = kind
						s.Signature = valueSignature(fset, kind, name.Name, sp, i)
						s.Doc = firstDoc(d.Doc, sp.Doc)
						add(s, name, name.Pos())
					}
				}
			}
		}
	}
	return out, nil
}

// typeKind separates structs and interfaces from plain named types, collecting members when asked.
func typeKind(fset *token.FileSet, sp *ast.TypeSpec, withMembers bool) (Kind, []Member) {
	switch t := sp.Type.(type) {
	case *ast.StructType:
		if !withMembers {
			return KindStruct, nil
		}
		return KindStruct, structMembers(fset, t)
	case *ast.InterfaceType:
		if !withMembers {
			return KindInterface, nil
		}
		return KindInterface, interfaceMembers(fset, t)
	}
	return KindType, nil
}

func structMembers(fset *token.FileSet, t *ast.StructType) []Member {
	var out []Member
	for _, f := range t.Fields.List {
		typ := exprString(fset, f.Type)
		doc := firstDoc(f.Doc, f.Comment)
		if len(f.Names) == 0 {
			out = append(out, Member{Name: typ, Type: typ, Doc: doc})
			continue
		}
		for _, n := range f.Names {
			out = append(out, Member{Name: n.Name, Type: typ, Doc: doc})
		}
	}
	return out
}

func interfaceMembers(fset *token.FileSet, t *ast.InterfaceType) []Member {
	var out []Member
	for _, f := range t.Methods.List {
		typ := exprString(fset, f.Type)
		if len(f.Names) == 0 {
			out = append(out, Member{Name: typ, Type: typ, Doc: docLine(f.Doc)})
			continue
		}
		for _, n := range f.Names {
			sig := strings.TrimPrefix(typ, "func")
			out = append(out, Member{Name: n.Name, Type: n.Name + sig, Doc: docLine(f.Doc)})
		}
	}
	return out
}

func funcSignature(fset *token.FileSet, d *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("func ")
	if recv := recvString(fset, d.Recv); recv != "" {
		b.WriteString("(" + recv + ") ")
	}
	b.WriteString(d.Name.Name)
	sig := exprString(fset, d.Type)
	b.WriteString(strings.TrimPrefix(sig, "func"))
	return b.String()
}

func typeSignature(fset *token.FileSet, sp *ast.TypeSpec) string {
	var b strings.Builder
	b.WriteString("type " + sp.Name.Name)
	if sp.TypeParams != nil && len(sp.TypeParams.List) > 0 {
		b.WriteString("[" + fieldList(fset, sp.TypeParams) + "]")
	}
	if sp.Assign.IsValid() {
		b.WriteString(" = ")
	} else {
		b.WriteString(" ")
	}
	switch sp.Type.(type) {
	case *ast.StructType:
		b.WriteString("struct")
	case *ast.InterfaceType:
		b.WriteString("interface")
	default:
		b.WriteString(exprString(fset, sp.Type))
	}
	return b.String()
}

// valueSignature renders one name out of a const/var spec, keeping its type or its initializer but not both.
func valueSignature(fset *token.FileSet, kind Kind, name string, sp *ast.ValueSpec, i int) string {
	b := string(kind) + " " + name
	if sp.Type != nil {
		return b + " " + exprString(fset, sp.Type)
	}
	if i < len(sp.Values) {
		return b + " = " + exprString(fset, sp.Values[i])
	}
	return b
}

func recvString(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	return fieldList(fset, fl)
}

func fieldList(fset *token.FileSet, fl *ast.FieldList) string {
	var parts []string
	for _, f := range fl.List {
		typ := exprString(fset, f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		for _, n := range f.Names {
			parts = append(parts, n.Name+" "+typ)
		}
	}
	return strings.Join(parts, ", ")
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, e); err != nil {
		return "?"
	}
	return buf.String()
}

// firstDoc returns the first non-empty doc line among the groups given.
func firstDoc(groups ...*ast.CommentGroup) string {
	for _, g := range groups {
		if line := docLine(g); line != "" {
			return line
		}
	}
	return ""
}

func docLine(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}
	text := strings.TrimSpace(cg.Text())
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	return strings.TrimSpace(text)
}
