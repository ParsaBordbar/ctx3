package flow

import (
	"path/filepath"
	"strings"
	"testing"
)

// impactFixture: main -> a -> leaf, main -> b -> leaf, plus a method calling leaf.
func impactFixture(t *testing.T) *CallGraph {
	t.Helper()
	td := t.TempDir()
	writeModule(t, td)
	writeGoFile(t, filepath.Join(td, "main.go"), `package main

type Store struct{}

func main() {
	a()
	b()
}

func a() { leaf() }

func b() { leaf() }

func (s *Store) Save() { leaf() }

func leaf() {}

func orphan() {}
`)

	g, err := AnalyzeFlow(Config{RootDir: td})
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}
	return g
}

func TestImpacted_TransitiveCallersAndDepth(t *testing.T) {
	g := impactFixture(t)

	imp, err := Impacted(g, "main.leaf", 0)
	if err != nil {
		t.Fatal(err)
	}

	depth := map[string]int{}
	for _, c := range imp.Callers {
		depth[c.Key] = c.Depth
	}
	for key, want := range map[string]int{
		"main.a": 1, "main.b": 1, "main.Store.Save": 1, "main.main": 2,
	} {
		if depth[key] != want {
			t.Errorf("%s depth = %d, want %d", key, depth[key], want)
		}
	}
	if _, ok := depth["main.orphan"]; ok {
		t.Error("orphan should not appear in the impact set")
	}

	shallow, err := Impacted(g, "main.leaf", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range shallow.Callers {
		if c.Key == "main.main" {
			t.Error("--depth 1 should stop before main.main")
		}
	}
}

func TestImpacted_ReportsEntriesAndPackages(t *testing.T) {
	g := impactFixture(t)

	imp, err := Impacted(g, "main.leaf", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Entries) != 1 || imp.Entries[0] != "main.main" {
		t.Errorf("Entries = %v, want [main.main]", imp.Entries)
	}
	if len(imp.Packages) != 1 || imp.Packages[0] != "main" {
		t.Errorf("Packages = %v, want [main]", imp.Packages)
	}
}

func TestMatch_ExactThenNameThenSubstring(t *testing.T) {
	g := impactFixture(t)

	if got := g.Match("main.leaf"); len(got) != 1 || got[0] != "main.leaf" {
		t.Errorf("exact key match = %v", got)
	}
	if got := g.Match("leaf"); len(got) != 1 || got[0] != "main.leaf" {
		t.Errorf("bare name match = %v", got)
	}
	if got := g.Match("Save"); len(got) != 1 || got[0] != "main.Store.Save" {
		t.Errorf("method name match = %v", got)
	}
	if got := g.Match("Stor"); len(got) != 1 || got[0] != "main.Store.Save" {
		t.Errorf("substring match = %v", got)
	}
}

func TestImpacted_UnknownSymbolIsAnError(t *testing.T) {
	g := impactFixture(t)
	if _, err := Impacted(g, "nope", 0); err == nil {
		t.Fatal("expected an error for an unmatched symbol")
	}
}

func TestImpacted_NoCallersRendersPlainly(t *testing.T) {
	g := impactFixture(t)

	imp, err := Impacted(g, "main.orphan", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Callers) != 0 {
		t.Fatalf("orphan has callers: %+v", imp.Callers)
	}
	if out := RenderImpactText(imp); !strings.Contains(out, "no callers in this module") {
		t.Errorf("RenderImpactText = %q", out)
	}
}

func TestRenderImpactMermaid_EdgesPointAtTarget(t *testing.T) {
	g := impactFixture(t)

	imp, err := Impacted(g, "main.leaf", 0)
	if err != nil {
		t.Fatal(err)
	}
	out := RenderImpactMermaid(imp)
	for _, want := range []string{"flowchart BT", "main_a --> main_leaf", "main_main --> main_a", "style main_leaf"} {
		if !strings.Contains(out, want) {
			t.Errorf("mermaid missing %q:\n%s", want, out)
		}
	}
}
