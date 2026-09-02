package diffctx

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/parsabordbar/ctx3/flow"
	"github.com/parsabordbar/ctx3/gitfacts"
	"github.com/parsabordbar/ctx3/internal/tokens"
	"github.com/parsabordbar/ctx3/symbols"
)

type Config struct {
	RootDir string
	Ref     string
	Budget  int
	Depth   int
}

type File struct {
	Path    string `json:"path"    toon:"path"`
	Status  string `json:"status"  toon:"status"`
	Added   int    `json:"added"   toon:"added"`
	Removed int    `json:"removed" toon:"removed"`
}

type Range struct {
	Start int `json:"start" toon:"start"`
	End   int `json:"end"   toon:"end"`
}

type Symbol struct {
	Name      string        `json:"name"      toon:"name"`
	Kind      symbols.Kind  `json:"kind"      toon:"kind"`
	Package   string        `json:"package"   toon:"package"`
	Signature string        `json:"signature" toon:"signature"`
	File      string        `json:"file"      toon:"file"`
	Line      int           `json:"line"      toon:"line"`
	Exported  bool          `json:"exported"  toon:"exported"`
	Hunks     []Range       `json:"hunks"     toon:"hunks"`
	Callers   []flow.Caller `json:"callers,omitempty" toon:"callers,omitempty"`
	Entries   []string      `json:"entries,omitempty" toon:"entries,omitempty"`
	Tests     []string      `json:"tests,omitempty"   toon:"tests,omitempty"`
	key       string
}

type Context struct {
	Root      string   `json:"root"      toon:"root"`
	Ref       string   `json:"ref"       toon:"ref"`
	Files     []File   `json:"files"     toon:"files"`
	Symbols   []Symbol `json:"symbols"   toon:"symbols"`
	Tests     []string `json:"tests"     toon:"tests"`
	Budget    int      `json:"budget"    toon:"budget"`
	Tokens    int      `json:"tokens"    toon:"tokens"`
	Truncated bool     `json:"truncated" toon:"truncated"`
	Degraded  bool     `json:"degraded"  toon:"degraded"`
	Notes     []string `json:"notes,omitempty" toon:"notes,omitempty"`
}

var hunkRe = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

func Build(cfg Config) (*Context, error) {
	root := cfg.RootDir
	if root == "" {
		root = "."
	}
	if cfg.Depth <= 0 {
		cfg.Depth = 3
	}
	if !gitfacts.IsRepo(root) {
		return nil, gitfacts.ErrNotARepo
	}
	ref := strings.TrimSpace(cfg.Ref)
	if ref == "" {
		ref = "HEAD"
	}
	if _, err := gitfacts.Run(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return nil, fmt.Errorf("unknown ref %q", ref)
	}

	ctx := &Context{Root: root, Ref: ref, Budget: cfg.Budget}
	changed := map[string][]Range{}
	status := map[string]string{}

	out, _ := gitfacts.Run(root, "diff", "--numstat", "--find-renames", ref, "--")
	for _, line := range strings.Split(out, "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		removed, _ := strconv.Atoi(parts[1])
		path := renameTarget(parts[2])
		st := "modified"
		switch {
		case removed == 0 && added > 0 && !existsIn(root, ref, path):
			st = "added"
		case added == 0 && removed > 0 && !fileExists(filepath.Join(root, path)):
			st = "deleted"
		}
		status[path] = st
		ctx.Files = append(ctx.Files, File{Path: path, Status: st, Added: added, Removed: removed})
		if st != "deleted" {
			changed[path] = hunks(root, ref, path)
		}
	}
	if untracked, err := gitfacts.Run(root, "ls-files", "--others", "--exclude-standard"); err == nil {
		for _, path := range strings.Split(strings.TrimSpace(untracked), "\n") {
			if path == "" {
				continue
			}
			n := countLines(filepath.Join(root, path))
			status[path] = "untracked"
			ctx.Files = append(ctx.Files, File{Path: path, Status: "untracked", Added: n})
			changed[path] = []Range{{Start: 1, End: n}}
		}
	}
	sort.Slice(ctx.Files, func(i, j int) bool { return ctx.Files[i].Path < ctx.Files[j].Path })
	if len(ctx.Files) == 0 {
		ctx.Notes = append(ctx.Notes, "no changes against "+ref)
		return ctx, nil
	}

	idx, err := symbols.Scan(symbols.Config{Path: root, IncludeUnexported: true, IncludeTests: true})
	if err != nil {
		return nil, err
	}
	byFile := map[string][]symbols.Symbol{}
	for _, s := range idx.Symbols {
		f := relTo(root, s.File)
		byFile[f] = append(byFile[f], s)
	}
	for path, ranges := range changed {
		syms := byFile[path]
		if len(syms) == 0 {
			continue
		}
		sort.Slice(syms, func(i, j int) bool { return syms[i].Line < syms[j].Line })
		for i, s := range syms {
			end := 1 << 30
			if i+1 < len(syms) {
				end = syms[i+1].Line - 1
			}
			var hit []Range
			for _, r := range ranges {
				if r.End >= s.Line && r.Start <= end {
					hit = append(hit, r)
				}
			}
			if len(hit) == 0 {
				continue
			}
			ctx.Symbols = append(ctx.Symbols, Symbol{
				Name: s.Name, Kind: s.Kind, Package: s.Package, Signature: s.Signature,
				File: path, Line: s.Line, Exported: s.Exported, Hunks: hit, key: graphKey(s),
			})
		}
	}
	sort.Slice(ctx.Symbols, func(i, j int) bool {
		if ctx.Symbols[i].File != ctx.Symbols[j].File {
			return ctx.Symbols[i].File < ctx.Symbols[j].File
		}
		return ctx.Symbols[i].Line < ctx.Symbols[j].Line
	})

	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && len(ctx.Symbols) > 0 {
		g, err := flow.AnalyzeFlow(flow.Config{RootDir: root})
		if err != nil {
			ctx.Notes = append(ctx.Notes, "call graph unavailable: "+err.Error())
		} else {
			ctx.Degraded = g.Degraded
			ctx.Notes = append(ctx.Notes, g.Notes...)
			for i := range ctx.Symbols {
				s := &ctx.Symbols[i]
				if s.Kind != symbols.KindFunc && s.Kind != symbols.KindMethod {
					continue
				}
				imp, err := flow.Impacted(g, s.key, cfg.Depth)
				if err != nil {
					continue
				}
				s.Callers = imp.Callers
				if len(s.Callers) > 20 {
					s.Callers = s.Callers[:20]
				}
				s.Entries = imp.Entries
			}
		}
	}

	findTests(root, ctx, status)
	fit(ctx)
	return ctx, nil
}

func renameTarget(p string) string {
	if i := strings.Index(p, " => "); i >= 0 {
		p = p[i+4:]
		p = strings.TrimSuffix(p, "}")
		if j := strings.Index(p, "{"); j >= 0 {
			p = p[:j] + p[j+1:]
		}
	}
	return p
}

func existsIn(root, ref, path string) bool {
	_, err := gitfacts.Run(root, "cat-file", "-e", ref+":"+path)
	return err == nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func countLines(p string) int {
	b, err := os.ReadFile(p)
	if err != nil || len(b) == 0 {
		return 0
	}
	n := strings.Count(string(b), "\n")
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

func hunks(root, ref, path string) []Range {
	out, err := gitfacts.Run(root, "diff", "-U0", ref, "--", path)
	if err != nil {
		return nil
	}
	var rs []Range
	for _, line := range strings.Split(out, "\n") {
		m := hunkRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		n := 1
		if m[2] != "" {
			n, _ = strconv.Atoi(m[2])
		}
		end := start + n - 1
		if n == 0 {
			end = start
		}
		rs = append(rs, Range{Start: start, End: end})
	}
	return rs
}

func graphKey(s symbols.Symbol) string {
	if s.Recv != "" {
		r := s.Recv
		if i := strings.LastIndexByte(r, ' '); i >= 0 {
			r = r[i+1:]
		}
		r = strings.TrimLeft(r, "*")
		if i := strings.IndexByte(r, '['); i >= 0 {
			r = r[:i]
		}
		return s.Package + "." + r + "." + s.Name
	}
	return s.Package + "." + s.Name
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "__pycache__": true}

func isTestFile(p string) bool {
	base := filepath.Base(p)
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".test.") ||
		strings.Contains(base, ".spec.") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") || strings.HasSuffix(base, "_test.rb") ||
		strings.HasSuffix(base, "_spec.rb")
}

func findTests(root string, ctx *Context, status map[string]string) {
	if len(ctx.Symbols) == 0 {
		return
	}
	names := map[string]*regexp.Regexp{}
	for _, s := range ctx.Symbols {
		if _, ok := names[s.Name]; !ok && len(s.Name) > 1 {
			names[s.Name] = regexp.MustCompile(`\b` + regexp.QuoteMeta(s.Name) + `\b`)
		}
	}
	seen := map[string]bool{}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTestFile(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel := relTo(root, path)
		for i := range ctx.Symbols {
			s := &ctx.Symbols[i]
			if s.File == rel {
				continue
			}
			if names[s.Name].Match(b) {
				s.Tests = append(s.Tests, rel)
				if !seen[rel] {
					seen[rel] = true
					ctx.Tests = append(ctx.Tests, rel)
				}
			}
		}
		return nil
	})
	for _, f := range ctx.Files {
		if isTestFile(f.Path) && !seen[f.Path] {
			seen[f.Path] = true
			ctx.Tests = append(ctx.Tests, f.Path)
		}
	}
	sort.Strings(ctx.Tests)
}

func fit(ctx *Context) {
	ctx.Tokens = tokens.Estimate(RenderText(ctx))
	if ctx.Budget <= 0 {
		return
	}
	for ctx.Tokens > ctx.Budget {
		trimmed := false
		for i := range ctx.Symbols {
			if len(ctx.Symbols[i].Callers) > 3 {
				ctx.Symbols[i].Callers = ctx.Symbols[i].Callers[:3]
				trimmed = true
			}
		}
		if !trimmed {
			break
		}
		ctx.Tokens = tokens.Estimate(RenderText(ctx))
		ctx.Truncated = true
	}
	for ctx.Tokens > ctx.Budget && len(ctx.Symbols) > 0 {
		ctx.Symbols = ctx.Symbols[:len(ctx.Symbols)-1]
		ctx.Tokens = tokens.Estimate(RenderText(ctx))
		ctx.Truncated = true
	}
	total := len(ctx.Files)
	for ctx.Tokens > ctx.Budget && len(ctx.Files) > 5 {
		ctx.Files = ctx.Files[:len(ctx.Files)/2]
		ctx.Tokens = tokens.Estimate(RenderText(ctx))
		ctx.Truncated = true
	}
	if len(ctx.Files) < total {
		ctx.Notes = append(ctx.Notes, fmt.Sprintf("%d of %d changed files listed", len(ctx.Files), total))
	}
}

func relTo(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(file))
}
