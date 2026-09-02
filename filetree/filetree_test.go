package filetree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tree materializes a directory layout from a path -> content map. A path
// ending in "/" is created as an empty directory.
func tree(t *testing.T, layout map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range layout {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestRender_SkipsIgnoredDirs(t *testing.T) {
	root := tree(t, map[string]string{
		"main.go":                  "package main\n",
		"node_modules/left-pad.js": "x",
		".git/config":              "x",
		"dist/bundle.js":           "x",
		"src/app.ts":               "x",
	})

	out := Render(Config{Root: root})
	for _, banned := range []string{"node_modules", ".git", "dist", "left-pad.js"} {
		if strings.Contains(out, banned) {
			t.Errorf("%s should be filtered out of:\n%s", banned, out)
		}
	}
	for _, want := range []string{"main.go", "src", "app.ts"} {
		if !strings.Contains(out, want) {
			t.Errorf("%s missing from:\n%s", want, out)
		}
	}
}

func TestRender_LastBranchGlyphIgnoresFilteredEntries(t *testing.T) {
	// "node_modules" sorts after "main.go", so filtering it inside the loop
	// would leave main.go drawn with a mid-tree "├──" and no last entry.
	root := tree(t, map[string]string{
		"main.go":           "x",
		"node_modules/x.js": "x",
		"node_modules/y.js": "x",
	})

	out := Render(Config{Root: root})
	if !strings.Contains(out, "└── main.go") {
		t.Errorf("main.go should be the last entry:\n%s", out)
	}
	if strings.Contains(out, "├── main.go") {
		t.Errorf("main.go drawn as a mid-tree entry:\n%s", out)
	}
}

func TestRender_MaxDepth(t *testing.T) {
	root := tree(t, map[string]string{
		"a/b/c/deep.go": "x",
		"top.go":        "x",
	})

	shallow := Render(Config{Root: root, MaxDepth: 1})
	if !strings.Contains(shallow, "a") || !strings.Contains(shallow, "top.go") {
		t.Errorf("depth 1 should show the top level:\n%s", shallow)
	}
	if strings.Contains(shallow, "deep.go") || strings.Contains(shallow, "b") {
		t.Errorf("depth 1 leaked deeper levels:\n%s", shallow)
	}

	if full := Render(Config{Root: root}); !strings.Contains(full, "deep.go") {
		t.Errorf("unlimited depth should reach deep.go:\n%s", full)
	}
}

func TestRender_Sizes(t *testing.T) {
	root := tree(t, map[string]string{"f.txt": "12345"})

	if out := Render(Config{Root: root, Sizes: true}); !strings.Contains(out, "f.txt (5 bytes)") {
		t.Errorf("size missing:\n%s", out)
	}
	// Sizes are opt-in: the default is names only.
	if out := Render(Config{Root: root}); strings.Contains(out, "bytes") {
		t.Errorf("sizes should be off by default:\n%s", out)
	}
}

func TestRender_UnreadableDirIsReportedNotFatal(t *testing.T) {
	out := Render(Config{Root: filepath.Join(t.TempDir(), "does-not-exist")})
	if !strings.Contains(out, "unreadable") {
		t.Errorf("a missing root should be reported, got %q", out)
	}
}

func TestRender_Deterministic(t *testing.T) {
	root := tree(t, map[string]string{
		"b.go": "x", "a.go": "x", "c/d.go": "x",
	})
	first := Render(Config{Root: root})
	for range 5 {
		if got := Render(Config{Root: root}); got != first {
			t.Fatalf("render is not deterministic:\n%s\n---\n%s", first, got)
		}
	}
}
