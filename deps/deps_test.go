package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffold writes a throwaway Go module with the given files (relpath -> content)
// and returns its root dir.
func scaffold(t *testing.T, module string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module " + module + "\n\ngo 1.22\n"
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAnalyze_InternalVsExternal(t *testing.T) {
	root := scaffold(t, "example.com/app", map[string]string{
		"main.go":    "package main\nimport (\n\t\"fmt\"\n\t\"example.com/app/store\"\n)\nfunc main() { fmt.Println(store.X) }\n",
		"store/s.go": "package store\nimport \"strings\"\nvar X = strings.ToUpper(\"x\")\n",
	})

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if g.Module != "example.com/app" {
		t.Fatalf("module = %q", g.Module)
	}
	if len(g.List) != 2 {
		t.Fatalf("want 2 packages, got %d", len(g.List))
	}

	main := g.Packages["example.com/app"]
	if len(main.Imports) != 1 || main.Imports[0] != "example.com/app/store" {
		t.Errorf("main internal imports = %v", main.Imports)
	}
	// fmt is external (stdlib), store is internal.
	if len(main.External) != 1 || main.External[0] != "fmt" {
		t.Errorf("main external imports = %v", main.External)
	}
}

func TestAnalyze_DetectsCycle(t *testing.T) {
	root := scaffold(t, "example.com/cyc", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/cyc/b\"\n",
		"b/b.go": "package b\nimport _ \"example.com/cyc/a\"\n",
	})

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Cycles) != 1 {
		t.Fatalf("want 1 cycle, got %d: %v", len(g.Cycles), g.Cycles)
	}
	if got := cycleKey(g.Cycles[0]); got != "example.com/cyc/a->example.com/cyc/b" {
		t.Errorf("cycle key = %q", got)
	}
}

func TestAnalyze_NoCycleWhenAcyclic(t *testing.T) {
	root := scaffold(t, "example.com/dag", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/dag/b\"\n",
		"b/b.go": "package b\n",
	})
	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Cycles) != 0 {
		t.Fatalf("expected acyclic, got cycles %v", g.Cycles)
	}
}

func TestAnalyze_TestFilesIgnored(t *testing.T) {
	root := scaffold(t, "example.com/tf", map[string]string{
		"a/a.go":      "package a\n",
		"a/a_test.go": "package a\nimport _ \"example.com/tf/b\"\n",
		"b/b.go":      "package b\n",
	})
	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	// b is only imported from a test file, so a must have no internal imports.
	if a := g.Packages["example.com/tf/a"]; len(a.Imports) != 0 {
		t.Errorf("test-file import leaked into graph: %v", a.Imports)
	}
}

func TestAnalyze_NoGoMod(t *testing.T) {
	root := t.TempDir()
	if _, err := Analyze(Config{RootDir: root}); err == nil {
		t.Fatal("want error when go.mod is absent")
	}
}

func TestRenderText_And_Mermaid(t *testing.T) {
	root := scaffold(t, "example.com/cyc", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/cyc/b\"\n",
		"b/b.go": "package b\nimport _ \"example.com/cyc/a\"\n",
	})
	g, _ := Analyze(Config{RootDir: root})

	txt := RenderText(g)
	if !strings.Contains(txt, "Circular imports detected") {
		t.Errorf("text render missing cycle warning:\n%s", txt)
	}
	mer := RenderMermaid(g)
	if !strings.Contains(mer, "flowchart LR") || !strings.Contains(mer, "class p_a cycle") {
		t.Errorf("mermaid render wrong:\n%s", mer)
	}
}
