package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestAnalyzeFlow_BasicCallGraph(t *testing.T) {
	td := t.TempDir()

	writeGoFile(t, filepath.Join(td, "main.go"), `package main

func main() {
	greet()
	run()
}

func greet() {
	format()
}

func run() {}
func format() {}
`)

	cfg := Config{RootDir: td}
	g, err := AnalyzeFlow(cfg)
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}

	if _, ok := g.Nodes["main.main"]; !ok {
		t.Fatalf("expected main.main in graph; got keys: %v", nodeKeys(g))
	}

	mainNode := g.Nodes["main.main"]
	if !mainNode.IsEntry {
		t.Fatalf("expected main.main to be an entry point")
	}
	if !contains(mainNode.Calls, "main.greet") {
		t.Fatalf("expected main.main to call main.greet; calls: %v", mainNode.Calls)
	}
	if !contains(mainNode.Calls, "main.run") {
		t.Fatalf("expected main.main to call main.run; calls: %v", mainNode.Calls)
	}

	greetNode := g.Nodes["main.greet"]
	if !contains(greetNode.Calls, "main.format") {
		t.Fatalf("expected main.greet to call main.format; calls: %v", greetNode.Calls)
	}
}

func TestAnalyzeFlow_MultiPackage(t *testing.T) {
	td := t.TempDir()

	writeGoFile(t, filepath.Join(td, "main.go"), `package main
import "fmt"
func main() { run() }
func run() { fmt.Println("hi") }
`)

	writeGoFile(t, filepath.Join(td, "analyzer", "analyze.go"), `package analyzer
func Analyze() { collect() }
func collect() {}
`)

	cfg := Config{RootDir: td}
	g, err := AnalyzeFlow(cfg)
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}

	if _, ok := g.Nodes["analyzer.Analyze"]; !ok {
		t.Fatalf("expected analyzer.Analyze; got %v", nodeKeys(g))
	}
	if _, ok := g.Nodes["analyzer.collect"]; !ok {
		t.Fatalf("expected analyzer.collect; got %v", nodeKeys(g))
	}

	// Packages collected
	if !containsStr(g.Packages, "main") || !containsStr(g.Packages, "analyzer") {
		t.Fatalf("expected both packages; got %v", g.Packages)
	}
}

func TestRenderMermaid_ContainsFlowchart(t *testing.T) {
	td := t.TempDir()
	writeGoFile(t, filepath.Join(td, "main.go"), `package main
func main() { helper() }
func helper() {}
`)
	cfg := Config{RootDir: td}
	g, err := AnalyzeFlow(cfg)
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}

	out := RenderMermaid(g)
	if !strings.HasPrefix(out, "```mermaid\nflowchart LR\n") {
		t.Fatalf("expected mermaid header; got:\n%s", out)
	}
	if !strings.Contains(out, "main_main") {
		t.Fatalf("expected node main_main in mermaid; got:\n%s", out)
	}
	if !strings.Contains(out, "main_main --> main_helper") {
		t.Fatalf("expected edge main_main --> main_helper; got:\n%s", out)
	}
	if !strings.HasSuffix(out, "```\n") {
		t.Fatalf("expected closing ``` fence; got:\n%s", out)
	}
}

func TestRenderText_ContainsTree(t *testing.T) {
	td := t.TempDir()
	writeGoFile(t, filepath.Join(td, "main.go"), `package main
func main() { helper() }
func helper() {}
`)
	cfg := Config{RootDir: td}
	g, err := AnalyzeFlow(cfg)
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}

	out := RenderText(g)
	if !strings.HasPrefix(out, "┌── Code Flow\n") {
		t.Fatalf("expected text header; got:\n%s", out)
	}
	if !strings.Contains(out, "main.main") {
		t.Fatalf("expected main.main in text output; got:\n%s", out)
	}
}

func TestMermaidID(t *testing.T) {
	cases := [][2]string{
		{"main.main", "main_main"},
		{"analyzer.Analyze", "analyzer_Analyze"},
		{"pack.WalkAndCollect", "pack_WalkAndCollect"},
	}
	for _, c := range cases {
		if got := mermaidID(c[0]); got != c[1] {
			t.Errorf("mermaidID(%q) = %q; want %q", c[0], got, c[1])
		}
	}
}

func nodeKeys(g *CallGraph) []string {
	ks := make([]string, 0, len(g.Nodes))
	for k := range g.Nodes {
		ks = append(ks, k)
	}
	return ks
}

func contains(ss []string, s string) bool {
	for _, e := range ss {
		if e == s {
			return true
		}
	}
	return false
}

func containsStr(ss []string, s string) bool { return contains(ss, s) }