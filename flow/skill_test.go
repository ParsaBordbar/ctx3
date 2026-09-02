package flow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scaffold(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = "module example.com/x\n\ngo 1.24\n"
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCallGraph_Skill(t *testing.T) {
	root := scaffold(t, map[string]string{
		"main.go": "package main\nfunc helper() {}\nfunc main() { helper() }\n",
	})
	g, err := AnalyzeFlow(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	s := g.Skill("myapp")

	if s.Name != "myapp-flow" {
		t.Errorf("skill name = %q, want myapp-flow", s.Name)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("generated skill fails validation: %v", err)
	}
	if !strings.Contains(s.Description, "call") && !strings.Contains(s.Description, "Call") {
		t.Errorf("description missing trigger keywords: %q", s.Description)
	}
	// Package map first (cheapest, most orienting), then the tree, then Mermaid.
	wantRefs := []string{"packages.md", "callgraph.md", "callgraph.mermaid.md"}
	if len(s.References) != len(wantRefs) {
		t.Fatalf("want %d references, got %d", len(wantRefs), len(s.References))
	}
	for i, want := range wantRefs {
		if s.References[i].Filename != want {
			t.Errorf("reference %d = %q, want %q", i, s.References[i].Filename, want)
		}
	}
	// Skills must be fact-only.
	if strings.Contains(s.Overview, "TODO") {
		t.Error("skill overview must not contain TODO markers")
	}
}

func TestCallGraph_Skill_EmptyName(t *testing.T) {
	g := &CallGraph{Nodes: map[string]*FuncNode{}}
	s := g.Skill("")
	if s.Name != "project-flow" {
		t.Errorf("empty name should fall back to project-flow, got %q", s.Name)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("fallback skill fails validation: %v", err)
	}
}
