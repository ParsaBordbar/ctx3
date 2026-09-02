package deps

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

// Non-Go import graphs are resolved from source text rather than a build. The
// unit of the graph stays the *directory*, which is what a Go package already
// is, so a mixed repo (Go service + TypeScript frontend) produces one graph
// instead of two disjoint ones and the cycle detector needs no changes.
//
// The cost is the same as everywhere else in ctx3: dynamic imports built from
// variables, and path aliases declared in a tsconfig this code does not read,
// resolve to nothing and are dropped rather than guessed.

// tsImportRE matches `from "x"`, `import "x"`, `require("x")` and
// `import("x")`. The specifier is the first non-empty capture.
var tsImportRE = regexp.MustCompile(
	`(?m)(?:\bfrom\s+|^\s*import\s+|\brequire\s*\(\s*|\bimport\s*\(\s*)['"]([^'"]+)['"]`)

// pyFromRE matches `from x.y import z`, including relative `from . import z`.
var pyFromRE = regexp.MustCompile(`(?m)^\s*from\s+([.\w]+)\s+import\b`)

// pyImportRE matches `import x.y` and `import x.y as z`.
var pyImportRE = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)`)

// tsExts are tried, in order, when resolving a specifier to a file.
var tsExts = []string{".ts", ".tsx", ".d.ts", ".js", ".jsx", ".mjs", ".cjs"}

// langOf classifies a source file, returning "" when it is not one this
// resolver understands.
func langOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs":
		return "ts"
	case ".py", ".pyi":
		return "py"
	}
	return ""
}

// collectLangFile records one non-Go file's imports against its directory.
func collectLangFile(path, root, module, lang string, g *Graph) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // unreadable files are skipped, not fatal
	}
	src := string(data)

	dir := filepath.Dir(path)
	rel, _ := filepath.Rel(root, dir)
	rel = filepath.ToSlash(rel)

	p := packageFor(g, module, rel, filepath.Base(rel))

	var specs []string
	switch lang {
	case "ts":
		for _, m := range tsImportRE.FindAllStringSubmatch(src, -1) {
			specs = append(specs, m[1])
		}
	case "py":
		for _, re := range []*regexp.Regexp{pyFromRE, pyImportRE} {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				specs = append(specs, m[1])
			}
		}
	}

	for _, spec := range specs {
		target, internal := resolveSpec(spec, root, rel, lang)
		switch {
		case !internal:
			p.External = append(p.External, spec)
		case target == rel:
			// A sibling file in the same directory is not a package edge.
		default:
			p.Imports = append(p.Imports, importPathFor(module, target))
		}
	}
	return nil
}

// packageFor finds or creates the package record for a directory.
func packageFor(g *Graph, module, rel, name string) *Package {
	imp := importPathFor(module, rel)
	if p := g.Packages[imp]; p != nil {
		return p
	}
	if rel == "." || rel == "" {
		name = moduleBase(module)
	}
	p := &Package{ImportPath: imp, Dir: rel, Name: name}
	g.Packages[imp] = p
	return p
}

// importPathFor builds the module-qualified path for a directory, matching the
// shape the Go collector produces.
func importPathFor(module, rel string) string {
	if rel == "." || rel == "" {
		return module
	}
	return module + "/" + rel
}

// resolveSpec maps an import specifier to the directory it refers to, and
// reports whether that directory is inside the project.
func resolveSpec(spec, root, fromDir, lang string) (dir string, internal bool) {
	if lang == "py" {
		return resolvePython(spec, root, fromDir)
	}
	return resolveTS(spec, root, fromDir)
}

// resolveTS handles relative specifiers and the two alias prefixes that are
// near-universal in bundler configs. A bare specifier is a package dependency.
func resolveTS(spec, root, fromDir string) (string, bool) {
	var target string
	switch {
	case strings.HasPrefix(spec, "./"), strings.HasPrefix(spec, "../"), spec == ".", spec == "..":
		target = path.Join(fromDir, spec)
	case strings.HasPrefix(spec, "@/"):
		target = strings.TrimPrefix(spec, "@/")
	case strings.HasPrefix(spec, "~/"):
		target = strings.TrimPrefix(spec, "~/")
	default:
		return "", false // node_modules
	}
	return locate(root, target)
}

// resolvePython handles both `from .x import y` (relative, leading dots) and
// `import a.b` (absolute, internal only when a top-level directory matches).
func resolvePython(spec, root, fromDir string) (string, bool) {
	if strings.HasPrefix(spec, ".") {
		up := 0
		for up < len(spec) && spec[up] == '.' {
			up++
		}
		// One dot is "this package"; each extra dot climbs one level.
		base := fromDir
		for i := 1; i < up; i++ {
			base = path.Dir(base)
		}
		rest := strings.ReplaceAll(spec[up:], ".", "/")
		return locate(root, path.Join(base, rest))
	}

	// An absolute import is ours only when its first segment is a directory in
	// the project; otherwise it is the standard library or a site package.
	head := spec
	if i := strings.IndexByte(head, '.'); i >= 0 {
		head = head[:i]
	}
	if info, err := os.Stat(filepath.Join(root, head)); err != nil || !info.IsDir() {
		return "", false
	}
	return locate(root, strings.ReplaceAll(spec, ".", "/"))
}

// locate turns a project-relative target into the directory that holds it: the
// directory itself when the target is one, otherwise the directory containing
// the module file the specifier names.
func locate(root, target string) (string, bool) {
	target = path.Clean(target)
	if target == "" || strings.HasPrefix(target, "..") {
		return "", false // escapes the project
	}

	if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err == nil && info.IsDir() {
		return target, true
	}
	for _, ext := range tsExts {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target+ext))); err == nil {
			return path.Dir(target), true
		}
	}
	for _, ext := range []string{".py", ".pyi"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target+ext))); err == nil {
			return path.Dir(target), true
		}
	}
	return "", false
}

// gitignoreFilter returns a predicate reporting whether a path is ignored by
// the repository's root .gitignore. It returns a permissive filter when there
// is no .gitignore to read.
func gitignoreFilter(root string) func(path string, isDir bool) bool {
	gi, err := ignore.CompileIgnoreFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return func(string, bool) bool { return false }
	}
	return func(p string, isDir bool) bool {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return false
		}
		rel = filepath.ToSlash(rel)
		if isDir {
			rel += "/"
		}
		return gi.MatchesPath(rel)
	}
}

// packageJSONName reads the "name" field of a package.json, if there is one.
func packageJSONName(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`"name"\s*:\s*"([^"]+)"`).FindSubmatch(data)
	if m == nil {
		return ""
	}
	// A scoped name (@org/pkg) keeps only its last element as the graph root.
	return string(m[1])
}
