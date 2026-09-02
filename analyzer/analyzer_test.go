package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestParseGoModRequires_Block(t *testing.T) {
	mod := `module example.com/x

go 1.24

require (
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/toon-format/toon-go v0.0.0-20251202084852-7ca0e27c4e8c
)
`
	deps, indirect := parseGoModRequires(mod)
	// Indirect requirements are counted, not listed: they are transitive noise.
	if indirect != 1 {
		t.Errorf("indirect count = %d, want 1", indirect)
	}
	want := []string{
		"github.com/bmatcuk/doublestar/v4 v4.9.1",
		"github.com/spf13/cobra v1.9.1",
		"github.com/toon-format/toon-go v0.0.0-20251202084852-7ca0e27c4e8c",
	}
	if len(deps) != len(want) {
		t.Fatalf("got %d deps, want %d: %v", len(deps), len(want), deps)
	}
	for i := range want {
		if deps[i] != want[i] {
			t.Errorf("dep %d = %q, want %q", i, deps[i], want[i])
		}
	}
	// The literal "(" must never leak in as a dependency.
	for _, d := range deps {
		if strings.Contains(d, "(") {
			t.Errorf("dependency contains stray paren: %q", d)
		}
	}
}

func TestParseGoModRequires_SingleLine(t *testing.T) {
	mod := "module example.com/x\n\ngo 1.24\n\nrequire github.com/spf13/cobra v1.9.1\n"
	deps, _ := parseGoModRequires(mod)
	if len(deps) != 1 || deps[0] != "github.com/spf13/cobra v1.9.1" {
		t.Fatalf("single-line require not parsed: %v", deps)
	}
}

func TestReadmePreview_SkipsBadgesAndKeepsProse(t *testing.T) {
	readme := "# Context Tree (ctx3)\n\n" +
		"[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)\n" +
		"[![Build](https://img.shields.io/x)](https://y)\n\n" +
		"ctx3 turns a repo into structured views an LLM can read.\n"

	got := readmePreview([]byte(readme), 400)
	if strings.Contains(got, "shields.io") {
		t.Errorf("badge leaked into preview: %q", got)
	}
	if !strings.Contains(got, "structured views an LLM can read") {
		t.Errorf("prose missing from preview: %q", got)
	}
	if !strings.Contains(got, "Context Tree (ctx3)") {
		t.Errorf("title missing from preview: %q", got)
	}
}

func TestReadmePreview_CutsOnRuneBoundary(t *testing.T) {
	got := readmePreview([]byte(strings.Repeat("é", 500)), 10)
	if !utf8.ValidString(got) {
		t.Fatalf("preview is not valid UTF-8: %q", got)
	}
}

// --- AnalyzeProject: the walk had no test at all before ---

// project materializes a directory layout from a path -> content map.
func project(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range layout {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestAnalyzeProject_SkipsNestedIgnoredDirs(t *testing.T) {
	// A nested node_modules is the common case in a polyglot repo, and matching
	// it only at the root let its contents dominate counts and percentages.
	root := project(t, map[string]string{
		"main.go":                   "package main\n",
		"web/node_modules/dep/a.js": "junk",
		"web/node_modules/dep/b.js": "junk",
		"services/api/vendor/v.go":  "junk",
		"web/dist/bundle.js":        "junk",
		"web/src/app.ts":            "export const x = 1\n",
	})

	ctx := AnalyzeProject(root)
	for _, f := range ctx.Files {
		for _, banned := range []string{"node_modules", "vendor", "dist"} {
			if strings.Contains(f.Path, banned) {
				t.Errorf("%s should have been skipped", f.Path)
			}
		}
	}
	if ctx.TotalFiles != 2 {
		var paths []string
		for _, f := range ctx.Files {
			paths = append(paths, f.Path)
		}
		t.Errorf("TotalFiles = %d, want 2 (%v)", ctx.TotalFiles, paths)
	}
}

func TestAnalyzeProject_EntryPointsAndLanguages(t *testing.T) {
	root := project(t, map[string]string{
		"main.go":    "package main\n\nfunc main() {}\n",
		"helper.go":  "package main\n",
		"readme.txt": "not a readme.md\n",
	})

	ctx := AnalyzeProject(root)

	var entries []string
	for _, f := range ctx.Files {
		if f.IsEntryPoint {
			entries = append(entries, f.Name)
		}
	}
	if len(entries) != 1 || entries[0] != "main.go" {
		t.Errorf("entry points = %v, want [main.go]", entries)
	}

	if len(ctx.Languages) == 0 {
		t.Fatal("no languages collected")
	}
	// Shares are by bytes and must sum to 100.
	var total float64
	for _, l := range ctx.Languages {
		total += l.Percent
	}
	if total < 99.9 || total > 100.1 {
		t.Errorf("language shares sum to %.2f, want 100", total)
	}
	// Largest share first.
	for i := 1; i < len(ctx.Languages); i++ {
		if ctx.Languages[i-1].Percent < ctx.Languages[i].Percent {
			t.Errorf("languages not sorted by share: %+v", ctx.Languages)
		}
	}
}

func TestAnalyzeProject_DirectDepsOnlyAndReadmePreview(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":    "module example.com/x\n\ngo 1.24\n\nrequire (\n\tgithub.com/spf13/cobra v1.9.1\n\tgithub.com/spf13/pflag v1.0.6 // indirect\n)\n",
		"README.md": "# Proj\n\n[![Build](https://img.shields.io/x)](https://y)\n\n---\n\nProj does a thing.\n",
	})

	ctx := AnalyzeProject(root)

	if len(ctx.Dependencies) != 1 || ctx.Dependencies[0] != "github.com/spf13/cobra v1.9.1" {
		t.Errorf("dependencies = %v, want only the direct one", ctx.Dependencies)
	}
	if ctx.IndirectCount != 1 {
		t.Errorf("IndirectCount = %d, want 1", ctx.IndirectCount)
	}
	if strings.Contains(ctx.Readme, "shields.io") || strings.Contains(ctx.Readme, "---") {
		t.Errorf("badges/rules leaked into README preview: %q", ctx.Readme)
	}
	if !strings.Contains(ctx.Readme, "Proj does a thing.") {
		t.Errorf("README prose missing: %q", ctx.Readme)
	}
}

func TestAnalyzeProject_LastEditedIsStableFormat(t *testing.T) {
	root := project(t, map[string]string{"a.go": "package a\n"})

	ctx := AnalyzeProject(root)
	if len(ctx.Files) != 1 {
		t.Fatalf("want 1 file, got %d", len(ctx.Files))
	}
	// RFC3339, not Go's default time formatting with its monotonic-clock tail.
	if _, err := time.Parse(time.RFC3339, ctx.Files[0].LastEdited); err != nil {
		t.Errorf("LastEdited %q is not RFC3339: %v", ctx.Files[0].LastEdited, err)
	}
}

func TestRenderPercentage_DeterministicAndPlain(t *testing.T) {
	pcts := map[string]float64{"go": 60, "md": 30, "xml": 10}

	first := RenderPercentage(pcts, false)
	for range 5 {
		if got := RenderPercentage(pcts, false); got != first {
			t.Fatalf("not deterministic:\n%s\n---\n%s", first, got)
		}
	}
	// Biggest share first, and no ANSI escapes when color is off.
	if strings.Index(first, "go") > strings.Index(first, "xml") {
		t.Errorf("not sorted by share:\n%s", first)
	}
	if strings.Contains(first, "\033[") {
		t.Errorf("color escapes present with color=false:\n%q", first)
	}
	if !strings.Contains(RenderPercentage(pcts, true), "\033[") {
		t.Error("color=true should emit escapes")
	}
}
