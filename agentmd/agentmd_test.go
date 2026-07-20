package agentmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
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
		"coolproj",           // project name from module path
		"## Commands",
		"go test ./...",
		"## Architecture",
		"main.main",          // entry from call-graph
		"CoolProj does a useful thing", // README blurb
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("generated doc missing %q", want)
		}
	}
}
