package brief

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/parsabordbar/ctx3/deps"
	"github.com/parsabordbar/ctx3/flow"
	"github.com/parsabordbar/ctx3/internal/tokens"
	"github.com/parsabordbar/ctx3/symbols"
)

type Config struct {
	RootDir    string
	Query      string
	Budget     int
	MaxSymbols int
	Depth      int
	Snippet    int
}

type Hit struct {
	Name      string        `json:"name"      toon:"name"`
	Kind      symbols.Kind  `json:"kind"      toon:"kind"`
	Package   string        `json:"package"   toon:"package"`
	Signature string        `json:"signature" toon:"signature"`
	Doc       string        `json:"doc,omitempty" toon:"doc,omitempty"`
	File      string        `json:"file"      toon:"file"`
	Line      int           `json:"line"      toon:"line"`
	Score     int           `json:"score"     toon:"score"`
	Source    string        `json:"source,omitempty"  toon:"source,omitempty"`
	Calls     []string      `json:"calls,omitempty"   toon:"calls,omitempty"`
	Callers   []flow.Caller `json:"callers,omitempty" toon:"callers,omitempty"`
	Entries   []string      `json:"entries,omitempty" toon:"entries,omitempty"`
	Imports   []string      `json:"imports,omitempty" toon:"imports,omitempty"`
	key       string
	nextLine  int
}

type Brief struct {
	Query     string   `json:"query"     toon:"query"`
	Root      string   `json:"root"      toon:"root"`
	Hits      []Hit    `json:"hits"      toon:"hits"`
	Budget    int      `json:"budget"    toon:"budget"`
	Tokens    int      `json:"tokens"    toon:"tokens"`
	Truncated bool     `json:"truncated" toon:"truncated"`
	Degraded  bool     `json:"degraded"  toon:"degraded"`
	Notes     []string `json:"notes,omitempty" toon:"notes,omitempty"`
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true, "this": true,
	"from": true, "into": true, "where": true, "what": true, "how": true, "does": true,
	"when": true, "are": true, "is": true, "of": true, "to": true, "in": true, "on": true,
	"a": true, "an": true, "do": true, "we": true, "it": true, "add": true, "fix": true,
	"make": true, "change": true, "update": true, "use": true, "get": true, "set": true,
	"new": true, "all": true, "any": true, "not": true, "can": true, "should": true,
	"func": true, "function": true, "method": true, "type": true, "struct": true,
}

var wordRe = regexp.MustCompile(`[A-Za-z0-9_]+`)

func Terms(query string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range wordRe.FindAllString(query, -1) {
		for _, part := range splitCamel(w) {
			p := strings.ToLower(part)
			if len(p) < 3 || stopWords[p] || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func splitCamel(w string) []string {
	var parts []string
	start := 0
	for i := 1; i < len(w); i++ {
		if w[i] >= 'A' && w[i] <= 'Z' && (w[i-1] < 'A' || w[i-1] > 'Z') {
			parts = append(parts, w[start:i])
			start = i
		}
	}
	parts = append(parts, w[start:])
	if len(parts) > 1 {
		parts = append(parts, w)
	}
	return parts
}

func score(s symbols.Symbol, query string, terms []string) int {
	name := strings.ToLower(s.Name)
	q := strings.ToLower(strings.TrimSpace(query))
	total := 0
	if name == q || strings.ToLower(s.Package+"."+s.Name) == q {
		total += 100
	}
	if s.Recv != "" && strings.ToLower(recvType(s.Recv)+"."+s.Name) == q {
		total += 100
	}
	doc := strings.ToLower(s.Doc)
	sig := strings.ToLower(s.Signature)
	file := strings.ToLower(s.File)
	for _, t := range terms {
		switch {
		case name == t:
			total += 40
		case strings.Contains(name, t):
			total += 20
		}
		if strings.Contains(doc, t) {
			total += 5
		}
		if strings.Contains(sig, t) {
			total += 3
		}
		if strings.Contains(file, t) {
			total += 2
		}
	}
	if total > 0 && s.Exported {
		total++
	}
	return total
}

func recvType(recv string) string {
	r := recv
	if i := strings.LastIndexByte(r, ' '); i >= 0 {
		r = r[i+1:]
	}
	r = strings.TrimLeft(r, "*")
	if i := strings.IndexByte(r, '['); i >= 0 {
		r = r[:i]
	}
	return r
}

func graphKey(s symbols.Symbol) string {
	if s.Recv != "" {
		return s.Package + "." + recvType(s.Recv) + "." + s.Name
	}
	return s.Package + "." + s.Name
}

func Build(cfg Config) (*Brief, error) {
	root := cfg.RootDir
	if root == "" {
		root = "."
	}
	if strings.TrimSpace(cfg.Query) == "" {
		return nil, fmt.Errorf("brief needs a query: a symbol name or a task description")
	}
	if cfg.MaxSymbols <= 0 {
		cfg.MaxSymbols = 8
	}
	if cfg.Depth <= 0 {
		cfg.Depth = 3
	}
	if cfg.Snippet <= 0 {
		cfg.Snippet = 40
	}

	idx, err := symbols.Scan(symbols.Config{Path: root, IncludeUnexported: true})
	if err != nil {
		return nil, err
	}
	terms := Terms(cfg.Query)
	b := &Brief{Query: cfg.Query, Root: root, Budget: cfg.Budget}

	var hits []Hit
	for _, s := range idx.Symbols {
		sc := score(s, cfg.Query, terms)
		if sc == 0 {
			continue
		}
		hits = append(hits, Hit{
			Name: s.Name, Kind: s.Kind, Package: s.Package, Signature: s.Signature,
			Doc: s.Doc, File: relTo(root, s.File), Line: s.Line, Score: sc, key: graphKey(s),
		})
	}
	if len(hits) == 0 {
		b.Notes = append(b.Notes, fmt.Sprintf("no symbol matched %q (terms: %s)", cfg.Query, strings.Join(terms, ", ")))
		return b, nil
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Line < hits[j].Line
	})
	if len(hits) > cfg.MaxSymbols {
		b.Notes = append(b.Notes, fmt.Sprintf("%d more symbols matched; raise --max to see them", len(hits)-cfg.MaxSymbols))
		hits = hits[:cfg.MaxSymbols]
	}

	byFile := map[string][]int{}
	for _, s := range idx.Symbols {
		f := relTo(root, s.File)
		byFile[f] = append(byFile[f], s.Line)
	}
	for f := range byFile {
		sort.Ints(byFile[f])
	}
	for i := range hits {
		h := &hits[i]
		h.nextLine = 0
		for _, ln := range byFile[h.File] {
			if ln > h.Line {
				h.nextLine = ln
				break
			}
		}
	}

	if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
		g, err := flow.AnalyzeFlow(flow.Config{RootDir: root})
		if err != nil {
			b.Notes = append(b.Notes, "call graph unavailable: "+err.Error())
		} else {
			b.Degraded = g.Degraded
			b.Notes = append(b.Notes, g.Notes...)
			enrichFlow(g, hits, cfg.Depth)
		}
		if dg, err := deps.Analyze(deps.Config{RootDir: root}); err == nil {
			enrichDeps(dg, hits)
		}
	}

	b.Hits = hits
	fitBudget(b, root, cfg.Snippet)
	return b, nil
}

func enrichFlow(g *flow.CallGraph, hits []Hit, depth int) {
	for i := range hits {
		h := &hits[i]
		if h.Kind != symbols.KindFunc && h.Kind != symbols.KindMethod {
			continue
		}
		node := g.Nodes[h.key]
		if node == nil {
			continue
		}
		h.Calls = append([]string(nil), node.Calls...)
		sort.Strings(h.Calls)
		if len(h.Calls) > 12 {
			h.Calls = h.Calls[:12]
		}
		imp, err := flow.Impacted(g, h.key, depth)
		if err != nil {
			continue
		}
		h.Callers = imp.Callers
		if len(h.Callers) > 20 {
			h.Callers = h.Callers[:20]
		}
		h.Entries = imp.Entries
	}
}

func enrichDeps(dg *deps.Graph, hits []Hit) {
	byDir := map[string]*deps.Package{}
	for _, p := range dg.List {
		byDir[filepath.ToSlash(p.Dir)] = p
	}
	for i := range hits {
		dir := filepath.ToSlash(filepath.Dir(hits[i].File))
		if p := byDir[dir]; p != nil {
			for _, imp := range p.Imports {
				hits[i].Imports = append(hits[i].Imports, dg.ShortPath(imp))
			}
		}
	}
}

func fitBudget(b *Brief, root string, snippet int) {
	for i := range b.Hits {
		b.Hits[i].Source = readSpan(filepath.Join(root, b.Hits[i].File), b.Hits[i].Line, b.Hits[i].nextLine, snippet)
	}
	b.Tokens = tokens.Estimate(RenderText(b))
	if b.Budget <= 0 {
		return
	}
	for b.Tokens > b.Budget && snippet > 8 {
		snippet /= 2
		for i := range b.Hits {
			b.Hits[i].Source = readSpan(filepath.Join(root, b.Hits[i].File), b.Hits[i].Line, b.Hits[i].nextLine, snippet)
		}
		b.Tokens = tokens.Estimate(RenderText(b))
		b.Truncated = true
	}
	for b.Tokens > b.Budget && len(b.Hits) > 1 {
		b.Hits = b.Hits[:len(b.Hits)-1]
		b.Tokens = tokens.Estimate(RenderText(b))
		b.Truncated = true
	}
	for b.Tokens > b.Budget && len(b.Hits) == 1 && b.Hits[0].Source != "" {
		b.Hits[0].Source = ""
		b.Hits[0].Callers = nil
		b.Tokens = tokens.Estimate(RenderText(b))
		b.Truncated = true
	}
}

func readSpan(path string, start, next, max int) string {
	if max <= 0 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	end := start + max - 1
	if next > 0 && next-1 < end {
		end = next - 1
	}
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	n := 0
	for sc.Scan() {
		n++
		if n < start {
			continue
		}
		if n > end {
			break
		}
		lines = append(lines, sc.Text())
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func relTo(root, file string) string {
	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(filepath.Clean(file))
}
