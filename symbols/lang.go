package symbols

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Non-Go languages are indexed by line-oriented pattern matching rather than a
// real parser. That is a deliberate trade: it keeps ctx3 dependency-free and
// preserves the promise the Go path already makes — a scan works on a partial
// checkout and on code that does not compile, which is exactly the state a repo
// is in while an agent is editing it. The cost is precision: a declaration
// written in an unusual style can be missed, and a name inside a string or a
// block comment can be picked up. The index is a navigation aid (name → kind →
// file:line), not a source of truth about semantics.

// container is an enclosing class/impl/trait block that turns the declarations
// inside it into methods.
type container struct {
	name string
	// scope is the brace depth (brace languages) or the indent column
	// (indent languages) at which the container was opened.
	scope int
}

// declRule maps one source line shape to a symbol kind.
type declRule struct {
	kind Kind
	re   *regexp.Regexp
	// opens marks a rule that starts a container: matches inside it become
	// methods with this declaration's name as the receiver.
	opens bool
	// fn marks a value binding that is really a function (`const f = () =>`).
	// It applies only when the line also looks like a function literal.
	fn bool
	// topOnly restricts a rule to module scope. Without it every `const x =`
	// inside a function body would be indexed as a top-level declaration.
	topOnly bool
	// needsContainer restricts a rule to the inside of a class or interface.
	// The method shape `name(args)` is indistinguishable from a plain call, so
	// outside a container it matches every `useEffect(...)` in a file.
	needsContainer bool
}

// langSpec describes how to read declarations out of one language.
type langSpec struct {
	name string
	// lineComment prefixes are used to harvest the doc line above a symbol.
	lineComment []string
	rules       []declRule
	// exported decides visibility from the declaration line.
	exported func(name, line string) bool
	// indentScoped selects indentation-based container tracking (Python, Ruby)
	// instead of brace counting.
	indentScoped bool
	// testFile reports whether a path is a test file for this language.
	testFile func(base string) bool
}

// ident is the name capture shared by every rule.
const ident = `(?P<name>[A-Za-z_$][A-Za-z0-9_$]*)`

// arrowFn recognizes a value binding whose initializer is a function.
var arrowFn = regexp.MustCompile(`=\s*(async\s+)?(function\b|\(|<|[A-Za-z_$][\w$]*\s*=>)`)

// notCall keeps control-flow keywords from being read as method declarations,
// since `if (x)` has the same shape as `name(args)`.
var notCall = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"return": true, "do": true, "else": true, "function": true, "new": true,
	"await": true, "typeof": true, "delete": true, "throw": true, "super": true,
	"constructor": false, // a real method — listed to document the exception
}

func exportedByKeyword(keyword string) func(string, string) bool {
	return func(_, line string) bool {
		return strings.HasPrefix(strings.TrimSpace(line), keyword) ||
			strings.Contains(line, " "+keyword+" ")
	}
}

// underscorePrivate is the Python/Ruby convention: a leading underscore means
// internal.
func underscorePrivate(name, _ string) bool { return !strings.HasPrefix(name, "_") }

var tsSpec = &langSpec{
	name:        "typescript",
	lineComment: []string{"//", "*"},
	rules: []declRule{
		{kind: KindFunc, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*` + ident)},
		{kind: KindStruct, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+` + ident), opens: true},
		{kind: KindInterface, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?interface\s+` + ident), opens: true},
		{kind: KindType, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?type\s+` + ident)},
		{kind: KindType, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:const\s+)?enum\s+` + ident), opens: true},
		{kind: KindConst, re: regexp.MustCompile(`^\s*(?:export\s+)?const\s+` + ident), fn: true, topOnly: true},
		{kind: KindVar, re: regexp.MustCompile(`^\s*(?:export\s+)?(?:let|var)\s+` + ident), fn: true, topOnly: true},
		{kind: KindMethod, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|readonly|abstract|get|set|\*)\s+)*` + ident + `\s*[(<]`), needsContainer: true},
	},
	exported: func(_, line string) bool {
		t := strings.TrimSpace(line)
		return strings.HasPrefix(t, "export") || !strings.HasPrefix(t, "private")
	},
	testFile: func(base string) bool {
		return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
	},
}

var pySpec = &langSpec{
	name:         "python",
	lineComment:  []string{"#"},
	indentScoped: true,
	rules: []declRule{
		{kind: KindFunc, re: regexp.MustCompile(`^\s*(?:async\s+)?def\s+` + ident)},
		{kind: KindStruct, re: regexp.MustCompile(`^\s*class\s+` + ident), opens: true},
		{kind: KindConst, re: regexp.MustCompile(`^(?P<name>[A-Z_][A-Z0-9_]*)\s*(?::[^=]+)?=`)},
		{kind: KindVar, re: regexp.MustCompile(`^(?P<name>[a-z_][A-Za-z0-9_]*)\s*(?::[^=]+)?=`)},
	},
	exported: underscorePrivate,
	testFile: func(base string) bool {
		return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
	},
}

var rustSpec = &langSpec{
	name:        "rust",
	lineComment: []string{"//", "///", "//!"},
	rules: []declRule{
		{kind: KindFunc, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const\s+)?(?:async\s+)?(?:unsafe\s+)?(?:extern\s+"[^"]*"\s+)?fn\s+` + ident)},
		{kind: KindStruct, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?struct\s+` + ident)},
		{kind: KindInterface, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:unsafe\s+)?trait\s+` + ident), opens: true},
		{kind: KindType, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?enum\s+` + ident)},
		{kind: KindType, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?type\s+` + ident)},
		{kind: KindConst, re: regexp.MustCompile(`^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const|static)\s+(?:mut\s+)?` + ident)},
		// `impl Trait for Type` and `impl Type` both scope methods to the type,
		// which is the name a reader searches for.
		{kind: "", re: regexp.MustCompile(`^\s*impl\b[^{]*?\bfor\s+(?P<name>[A-Za-z_][A-Za-z0-9_]*)`), opens: true},
		{kind: "", re: regexp.MustCompile(`^\s*impl(?:<[^>]*>)?\s+(?P<name>[A-Za-z_][A-Za-z0-9_]*)`), opens: true},
	},
	exported: exportedByKeyword("pub"),
	testFile: func(base string) bool { return strings.HasSuffix(base, "_test.rs") },
}

var javaSpec = &langSpec{
	name:        "java",
	lineComment: []string{"//", "*"},
	rules: []declRule{
		{kind: KindStruct, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final|abstract|sealed)\s+)*class\s+` + ident), opens: true},
		{kind: KindStruct, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final)\s+)*record\s+` + ident), opens: true},
		{kind: KindInterface, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|abstract|sealed)\s+)*interface\s+` + ident), opens: true},
		{kind: KindType, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final)\s+)*enum\s+` + ident), opens: true},
		// A method needs at least one modifier or a return type before the name,
		// which is what separates it from a bare call.
		{kind: KindMethod, re: regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|final|abstract|synchronized|native|default)\s+)+(?:<[^>]+>\s*)?(?:[\w.$<>,\[\]?\s]+\s+)?` + ident + `\s*\(`), needsContainer: true},
	},
	exported: func(_, line string) bool { return !strings.Contains(line, "private ") },
	testFile: func(base string) bool {
		return strings.HasSuffix(base, "Test.java") || strings.HasPrefix(base, "Test")
	},
}

var rubySpec = &langSpec{
	name:         "ruby",
	lineComment:  []string{"#"},
	indentScoped: true,
	rules: []declRule{
		{kind: KindFunc, re: regexp.MustCompile(`^\s*def\s+(?:self\.)?(?P<name>[A-Za-z_][A-Za-z0-9_]*[?!=]?)`)},
		{kind: KindStruct, re: regexp.MustCompile(`^\s*class\s+(?P<name>[A-Z][A-Za-z0-9_:]*)`), opens: true},
		{kind: KindType, re: regexp.MustCompile(`^\s*module\s+(?P<name>[A-Z][A-Za-z0-9_:]*)`), opens: true},
		{kind: KindConst, re: regexp.MustCompile(`^(?P<name>[A-Z][A-Z0-9_]*)\s*=`)},
	},
	exported: underscorePrivate,
	testFile: func(base string) bool {
		return strings.HasSuffix(base, "_spec.rb") || strings.HasSuffix(base, "_test.rb")
	},
}

// langByExt maps a file extension to its spec. Go is absent on purpose: it goes
// through the real parser in symbols.go.
var langByExt = map[string]*langSpec{
	".ts": tsSpec, ".tsx": tsSpec, ".mts": tsSpec, ".cts": tsSpec,
	".js": tsSpec, ".jsx": tsSpec, ".mjs": tsSpec, ".cjs": tsSpec,
	".py": pySpec, ".pyi": pySpec,
	".rs":   rustSpec,
	".java": javaSpec,
	".rb":   rubySpec,
}

// Languages lists every language the scanner indexes, sorted. "go" leads
// because it is the only one read by a real parser.
func Languages() []string {
	seen := map[string]bool{"go": true}
	out := []string{"go"}
	for _, spec := range langByExt {
		if !seen[spec.name] {
			seen[spec.name] = true
			out = append(out, spec.name)
		}
	}
	sort.Strings(out[1:])
	return out
}

// LanguageExts lists every non-Go extension the scanner understands, sorted.
func LanguageExts() []string {
	out := make([]string, 0, len(langByExt))
	for ext := range langByExt {
		out = append(out, ext)
	}
	sort.Strings(out)
	return out
}

// maxScanBytes and maxScanLine keep generated output from polluting the index.
// A minified or bundled file is one enormous line of code no human will ever
// navigate to, and pattern-matching it yields nothing but noise.
const (
	maxScanBytes = 1 << 20
	maxScanLine  = 1000
)

// scanLangFile extracts declarations from a non-Go source file.
func scanLangFile(path string, spec *langSpec, cfg Config) ([]Symbol, error) {
	base := filepath.Base(path)
	if spec.testFile != nil && spec.testFile(base) && !cfg.IncludeTests {
		return nil, nil
	}
	if isGenerated(base) {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > maxScanBytes {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dir := filepath.ToSlash(filepath.Dir(path))
	tmpl := Symbol{
		Package: filepath.Base(dir),
		Lang:    spec.name,
		Dir:     dir,
		File:    filepath.ToSlash(path),
	}

	var (
		out   []Symbol
		stack []container
		depth int
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	)

	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		// A blank line carries no scope: in an indent language its indent reads
		// as column 0, which would close every open class. Comments are skipped
		// for the same reason, and because a brace inside one is not real.
		if trimmed == "" || isCommentLine(trimmed, spec.lineComment) || len(trimmed) > maxScanLine {
			continue
		}

		scope := depth
		if spec.indentScoped {
			scope = indentOf(line)
		}

		// Leave any container the current line has stepped back out of. In a
		// brace language the body sits one depth below the declaration; in an
		// indent language it sits at a greater column. Both mean the container
		// ends as soon as the scope returns to where it was opened.
		for len(stack) > 0 && scope <= stack[len(stack)-1].scope {
			stack = stack[:len(stack)-1]
		}

		if rule, name, ok := matchDecl(spec, line, trimmed, len(stack) > 0); ok && !(rule.topOnly && scope > 0) {
			sym := tmpl
			sym.Name = name
			sym.Kind = rule.kind
			sym.Signature = squeeze(trimmed)
			sym.Line = i + 1
			sym.Doc = docAbove(lines, i, spec.lineComment)
			sym.Exported = spec.exported == nil || spec.exported(name, line)
			if rule.fn && arrowFn.MatchString(trimmed) {
				sym.Kind = KindFunc
			}
			if len(stack) > 0 {
				sym.Recv = stack[len(stack)-1].name
				if sym.Kind == KindFunc {
					sym.Kind = KindMethod
				}
			}
			// A container rule with no kind (Rust `impl`) only opens scope.
			if sym.Kind != "" && keepSymbol(sym, cfg) {
				out = append(out, sym)
			}
			if rule.opens {
				stack = append(stack, container{name: name, scope: scope})
			}
		}

		if !spec.indentScoped {
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth < 0 {
				depth = 0
			}
		}
	}
	return out, nil
}

// matchDecl finds the first rule that claims a line. Rule order is the
// precedence: specific declaration forms are listed before the catch-all
// method shape, so `function f()` never reads as a method call.
func matchDecl(spec *langSpec, line, trimmed string, inContainer bool) (declRule, string, bool) {
	for _, r := range spec.rules {
		if r.needsContainer && !inContainer {
			continue
		}
		m := r.re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := namedGroup(r.re, m)
		if name == "" {
			continue
		}
		if r.kind == KindMethod && isNotADecl(trimmed, name) {
			continue
		}
		return r, name, true
	}
	return declRule{}, "", false
}

// isNotADecl rejects the method-shaped lines that are really statements.
func isNotADecl(trimmed, name string) bool {
	if v, listed := notCall[name]; listed {
		return v
	}
	// A call used as a statement or an argument ends in `;`, `,` or `)`,
	// whereas a declaration continues into a body.
	switch {
	case strings.HasSuffix(trimmed, ";"), strings.HasSuffix(trimmed, ","):
		return true
	case strings.HasPrefix(trimmed, "."), strings.HasPrefix(trimmed, "}"):
		return true
	}
	return false
}

// namedGroup pulls the "name" capture out of a match.
func namedGroup(re *regexp.Regexp, m []string) string {
	for i, n := range re.SubexpNames() {
		if n == "name" && i < len(m) {
			return m[i]
		}
	}
	return ""
}

// keepSymbol applies the visibility, name and kind filters shared with the Go path.
func keepSymbol(s Symbol, cfg Config) bool {
	if !s.Exported && !cfg.IncludeUnexported {
		return false
	}
	if cfg.Match != nil && !cfg.Match.MatchString(s.Name) {
		return false
	}
	return true
}

// docAbove returns the comment line immediately above a declaration, skipping
// decorators and annotations, which sit between the doc and the symbol.
func docAbove(lines []string, at int, prefixes []string) string {
	for i := at - 1; i >= 0 && i >= at-8; i-- {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "", strings.HasPrefix(t, "@"), t == "*/", strings.HasPrefix(t, "/**"):
			continue
		case isCommentLine(t, prefixes):
			for _, p := range prefixes {
				if rest, ok := strings.CutPrefix(t, p); ok {
					t = strings.TrimSpace(rest)
					break
				}
			}
			if t == "" {
				continue
			}
			return squeeze(t)
		default:
			return ""
		}
	}
	return ""
}

func isCommentLine(trimmed string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}
	return strings.HasPrefix(trimmed, "/*")
}

// isGenerated recognizes the build artifacts that carry no navigable symbols.
func isGenerated(base string) bool {
	for _, suffix := range []string{".min.js", ".min.ts", ".bundle.js", "-min.js", ".d.ts"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

func indentOf(line string) int {
	n := 0
	for _, r := range line {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 4
		default:
			return n
		}
	}
	return n
}

// squeeze collapses runs of whitespace and drops a trailing brace so a
// signature reads as one line.
func squeeze(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "{"))
	return strings.Join(strings.Fields(s), " ")
}
