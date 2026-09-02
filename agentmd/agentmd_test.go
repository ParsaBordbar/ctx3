package agentmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	// Nested paths ("store/store.go") need their parent created first.
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectCommands_Go(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")

	groups := DetectCommands(dir)
	if len(groups) != 1 || groups[0].Title != "Go" {
		t.Fatalf("want a single Go group, got %+v", groups)
	}
	joined := strings.Join(groups[0].Lines, "\n")
	if !strings.Contains(joined, "go build") || !strings.Contains(joined, "go test") {
		t.Errorf("Go group missing build/test commands: %s", joined)
	}
}

func TestDetectCommands_NpmScripts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"scripts":{"build":"tsc","test":"jest"}}`)

	groups := DetectCommands(dir)
	if len(groups) != 1 || groups[0].Title != "Node / npm" {
		t.Fatalf("want a single npm group, got %+v", groups)
	}
	joined := strings.Join(groups[0].Lines, "\n")
	// Scripts are emitted sorted: build before test.
	if !strings.Contains(joined, "npm run build") || !strings.Contains(joined, "npm run test") {
		t.Errorf("npm group missing detected scripts: %s", joined)
	}
}

func TestMakeTargets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Makefile", "build:\n\tgo build\n\n.PHONY: build\n\ntest: build\n\tgo test\n")

	targets := makeTargets(dir)
	if len(targets) != 2 || targets[0] != "build" || targets[1] != "test" {
		t.Fatalf("want [build test], got %v", targets)
	}
}

func TestGenerate_CoreSections(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/coolproj\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "README.md", "# CoolProj\n\nCoolProj does a useful thing for everyone.\n")

	doc, err := Generate(Config{RootDir: dir, Title: "AGENTS.md"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# AGENTS.md",
		"coolproj", // project name from module path
		"## Commands",
		"go test ./...",
		"## Architecture",
		"main.main",                    // entry from call-graph
		"CoolProj does a useful thing", // README blurb
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("generated doc missing %q", want)
		}
	}
}

// The Architecture section used to emit "TODO: describe responsibility" for
// every package. These are facts ctx3 already computes, so they belong in the
// scaffold rather than being left for a human to restate.
func TestGenerate_ArchitectureCarriesRealFacts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/arch\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nimport \"example.com/arch/store\"\n\nfunc main() { store.Open() }\n")
	writeFile(t, dir, "store/store.go", "package store\n\n// Open opens it.\nfunc Open() {}\n")

	doc, err := Generate(Config{RootDir: dir, Title: "AGENTS.md"})
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(doc, "TODO: describe responsibility") {
		t.Error("per-package TODO placeholder should be gone")
	}
	if !strings.Contains(doc, "exported symbol(s)") {
		t.Errorf("architecture should report exported surface:\n%s", doc)
	}
	if !strings.Contains(doc, "Imports `store`") {
		t.Errorf("architecture should report internal imports:\n%s", doc)
	}
	// store imports nothing internal, so it reads in isolation.
	if !strings.Contains(doc, "Self-contained") {
		t.Errorf("architecture should name the leaf packages:\n%s", doc)
	}
}

func TestGenerate_ArchitectureReportsImportCycles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/cyc\n\ngo 1.22\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "a/a.go", "package a\n\nimport _ \"example.com/cyc/b\"\n")
	writeFile(t, dir, "b/b.go", "package b\n\nimport _ \"example.com/cyc/a\"\n")

	doc, err := Generate(Config{RootDir: dir, Title: "AGENTS.md"})
	if err != nil {
		t.Fatal(err)
	}
	// A cycle is the one architectural fact a reader cannot find in one file.
	if !strings.Contains(doc, "Import cycles") {
		t.Errorf("architecture should surface import cycles:\n%s", doc)
	}
}
