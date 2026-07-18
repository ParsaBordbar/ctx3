package deps

import (
	"strings"
	"testing"
)

func TestGraph_Skill(t *testing.T) {
	root := scaffold(t, "example.com/myapp", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/myapp/b\"\n",
		"b/b.go": "package b\n",
	})
	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	s := g.Skill()

	if s.Name != "myapp-deps" {
		t.Errorf("skill name = %q, want myapp-deps", s.Name)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("generated skill fails validation: %v", err)
	}
	if !strings.Contains(s.Description, "import cycles") {
		t.Errorf("description missing trigger keywords: %q", s.Description)
	}
	if len(s.References) != 2 {
		t.Fatalf("want 2 references, got %d", len(s.References))
	}
	// No TODO markers — skills must be fact-only.
	if strings.Contains(s.Overview, "TODO") {
		t.Error("skill overview must not contain TODO markers")
	}
}

func TestGraph_Skill_CycleNote(t *testing.T) {
	root := scaffold(t, "example.com/cyc", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/cyc/b\"\n",
		"b/b.go": "package b\nimport _ \"example.com/cyc/a\"\n",
	})
	g, _ := Analyze(Config{RootDir: root})
	s := g.Skill()
	if !strings.Contains(s.Overview, "circular import(s) detected") {
		t.Errorf("cycle note missing from overview: %q", s.Overview)
	}
}
