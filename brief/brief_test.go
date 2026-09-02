package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	write(t, dir, "main.go", "package main\n\nimport \"example.com/app/store\"\n\nfunc main() {\n\tstore.Open(\"x\")\n}\n")
	write(t, dir, "store/store.go", `package store

import "fmt"

// Open opens the named store and validates its path.
func Open(name string) error {
	if name == "" {
		return fmt.Errorf("empty")
	}
	return validate(name)
}

func validate(name string) error { return nil }

// Close is unrelated.
func Close() {}
`)
	return dir
}

func TestTerms(t *testing.T) {
	got := Terms("where do we validate the SkillName for a store")
	want := "validate skill name skillname store"
	if strings.Join(got, " ") != want {
		t.Fatalf("got %q want %q", strings.Join(got, " "), want)
	}
}

func TestBuild_RanksExactMatchAndFindsCallers(t *testing.T) {
	dir := fixture(t)
	b, err := Build(Config{RootDir: dir, Query: "Open"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Hits) == 0 || b.Hits[0].Name != "Open" {
		t.Fatalf("expected Open first, got %+v", b.Hits)
	}
	h := b.Hits[0]
	if !strings.Contains(h.Source, "func Open(name string) error") || strings.Contains(h.Source, "func validate") {
		t.Fatalf("source span wrong:\n%s", h.Source)
	}
	if len(h.Callers) != 1 || h.Callers[0].Key != "main.main" {
		t.Fatalf("callers: %+v", h.Callers)
	}
	if len(h.Calls) == 0 || !strings.Contains(strings.Join(h.Calls, ","), "store.validate") {
		t.Fatalf("calls: %v", h.Calls)
	}
	if len(h.Entries) != 1 || h.Entries[0] != "main.main" {
		t.Fatalf("entries: %v", h.Entries)
	}
	out := RenderText(b)
	if !strings.Contains(out, "store.Open") || !strings.Contains(out, "main.main 🚀") {
		t.Fatalf("render:\n%s", out)
	}
}

func TestBuild_FreeTextMatchesDoc(t *testing.T) {
	dir := fixture(t)
	b, err := Build(Config{RootDir: dir, Query: "validates its path"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Hits) == 0 || b.Hits[0].Name != "Open" {
		t.Fatalf("expected doc match on Open, got %+v", b.Hits)
	}
}

func TestBuild_BudgetTrims(t *testing.T) {
	dir := fixture(t)
	full, err := Build(Config{RootDir: dir, Query: "store"})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Hits) < 2 {
		t.Fatalf("fixture should match several symbols, got %d", len(full.Hits))
	}
	small, err := Build(Config{RootDir: dir, Query: "store", Budget: 60})
	if err != nil {
		t.Fatal(err)
	}
	if !small.Truncated || small.Tokens > 60 || len(small.Hits) >= len(full.Hits) {
		t.Fatalf("budget not honored: truncated=%v tokens=%d hits=%d/%d", small.Truncated, small.Tokens, len(small.Hits), len(full.Hits))
	}
}

func TestBuild_NoMatchIsNotAnError(t *testing.T) {
	dir := fixture(t)
	b, err := Build(Config{RootDir: dir, Query: "zzzzzz"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Hits) != 0 || len(b.Notes) == 0 {
		t.Fatalf("expected empty brief with a note, got %+v", b)
	}
}
