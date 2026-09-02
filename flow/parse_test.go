package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeParseModule materializes a throwaway module from a path -> source map.
func writeParseModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// brokenModule does not type-check: store.Save assigns a string to an int.
func brokenModule() map[string]string {
	return map[string]string{
		"go.mod": "module example.com/broken\n\ngo 1.24\n",
		"main.go": `package main

import "example.com/broken/store"

func main() {
	s := store.New()
	s.Save("x")
	helper()
}

func helper() {}
`,
		"store/store.go": `package store

type Store struct{}

func New() *Store { return &Store{} }

func (s *Store) Save(k string) {
	var n int = "not an int"
	_ = n
	s.flush()
}

func (s *Store) flush() {}
`,
	}
}

func TestAnalyzeFlow_FallsBackOnTypeError(t *testing.T) {
	dir := writeParseModule(t, brokenModule())

	g, err := AnalyzeFlow(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("a tree that does not compile must still analyze: %v", err)
	}
	if !g.Degraded {
		t.Error("graph should be marked degraded")
	}
	if len(g.Notes) == 0 {
		t.Error("degraded graph should explain why")
	}

	for _, key := range []string{"main.main", "main.helper", "store.New", "store.Store.Save", "store.Store.flush"} {
		if _, ok := g.Nodes[key]; !ok {
			t.Errorf("%s missing from parse-only graph", key)
		}
	}

	// Cross-package qualified call, same-package bare call, and a method call
	// on a value all have to resolve.
	main := g.Nodes["main.main"]
	for _, want := range []string{"store.New", "store.Store.Save", "main.helper"} {
		if !hasCall(main.Calls, want) {
			t.Errorf("main.main should call %s, has %v", want, main.Calls)
		}
	}
	if save := g.Nodes["store.Store.Save"]; !hasCall(save.Calls, "store.Store.flush") {
		t.Errorf("Save should call flush, has %v", save.Calls)
	}
	if len(g.Entries) != 1 || g.Entries[0] != "main.main" {
		t.Errorf("entries = %v, want [main.main]", g.Entries)
	}
}

func TestAnalyzeFlow_FallsBackOnSyntaxError(t *testing.T) {
	files := brokenModule()
	files["store/half.go"] = "package store\n\nfunc Halfwritten( {\n   not go at all\n"

	g, err := AnalyzeFlow(Config{RootDir: writeParseModule(t, files)})
	if err != nil {
		t.Fatalf("an unparseable file must not sink the scan: %v", err)
	}
	if !g.Degraded {
		t.Error("graph should be marked degraded")
	}
	// The file that cannot be parsed is skipped; everything else survives.
	if _, ok := g.Nodes["main.main"]; !ok {
		t.Error("main.main missing after a sibling file failed to parse")
	}
}

func TestAnalyzeFlow_CleanTreeIsNotDegraded(t *testing.T) {
	dir := writeParseModule(t, map[string]string{
		"go.mod":  "module example.com/clean\n\ngo 1.24\n",
		"main.go": "package main\n\nfunc main() { helper() }\n\nfunc helper() {}\n",
	})

	g, err := AnalyzeFlow(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if g.Degraded {
		t.Errorf("a compiling tree must use the type-checked path: %v", g.Notes)
	}
}

func TestParseOnly_AmbiguousMethodIsDropped(t *testing.T) {
	// Two types declare Close, so `x.Close()` cannot be attributed by name.
	// Inventing either edge would be worse than reporting none.
	dir := writeParseModule(t, map[string]string{
		"go.mod": "module example.com/amb\n\ngo 1.24\n",
		"main.go": `package main

type A struct{}
type B struct{}

func (a *A) Close() {}
func (b *B) Close() {}

func run(a *A) {
	a.Close()
}

func main() { run(nil) }
`,
	})

	g, err := analyzeParseOnly(dir, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := g.Nodes["main.run"]
	if run == nil {
		t.Fatal("main.run missing")
	}
	for _, c := range run.Calls {
		if strings.HasSuffix(c, ".Close") {
			t.Errorf("ambiguous method call was guessed: %v", run.Calls)
		}
	}
	if len(g.Notes) == 0 || !strings.Contains(strings.Join(g.Notes, " "), "more than one type") {
		t.Errorf("the dropped edge should be reported, notes = %v", g.Notes)
	}
}

func TestParseOnly_IgnoresOutOfModuleCalls(t *testing.T) {
	dir := writeParseModule(t, map[string]string{
		"go.mod": "module example.com/ext\n\ngo 1.24\n",
		"main.go": `package main

import (
	"fmt"
	"strings"
)

func main() {
	fmt.Println(strings.ToUpper("x"))
	local()
}

func local() {}
`,
	})

	g, err := analyzeParseOnly(dir, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	main := g.Nodes["main.main"]
	if got := main.Calls; len(got) != 1 || got[0] != "main.local" {
		t.Errorf("stdlib calls leaked into the graph: %v", got)
	}
}

func TestParseOnly_ImportAliasResolves(t *testing.T) {
	dir := writeParseModule(t, map[string]string{
		"go.mod": "module example.com/alias\n\ngo 1.24\n",
		"main.go": `package main

import st "example.com/alias/store"

func main() { st.New() }
`,
		"store/store.go": "package store\n\nfunc New() {}\n",
	})

	g, err := analyzeParseOnly(dir, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if main := g.Nodes["main.main"]; !hasCall(main.Calls, "store.New") {
		t.Errorf("aliased import did not resolve: %v", main.Calls)
	}
}

func TestParseOnly_VarClosureIsAnEntry(t *testing.T) {
	// The cobra shape: a package-level var holding a func literal that nothing
	// calls statically.
	dir := writeParseModule(t, map[string]string{
		"go.mod": "module example.com/clo\n\ngo 1.24\n",
		"main.go": `package main

var runE = func() error {
	doWork()
	return nil
}

func doWork() {}

func main() {}
`,
	})

	g, err := analyzeParseOnly(dir, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	node := g.Nodes["main.runE"]
	if node == nil {
		t.Fatal("var closure not captured")
	}
	if !node.IsEntry {
		t.Error("var closure should be an entry point")
	}
	if !hasCall(node.Calls, "main.doWork") {
		t.Errorf("closure calls = %v", node.Calls)
	}
}

func hasCall(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
