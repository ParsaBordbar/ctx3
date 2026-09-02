package diffctx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parsabordbar/ctx3/gitfacts"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)
	c := exec.Command("git", full...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

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

func repo(t *testing.T) string {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	write(t, dir, "go.mod", "module example.com/app\n\ngo 1.22\n")
	write(t, dir, "main.go", "package main\n\nimport \"example.com/app/store\"\n\nfunc main() {\n\tstore.Open(\"x\")\n}\n")
	write(t, dir, "store/store.go", "package store\n\nfunc Open(name string) error {\n\treturn nil\n}\n\nfunc Close() {}\n")
	write(t, dir, "store/store_test.go", "package store\n\nimport \"testing\"\n\nfunc TestOpen(t *testing.T) { Open(\"a\") }\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	return dir
}

func TestBuild_CleanTree(t *testing.T) {
	dir := repo(t)
	c, err := Build(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Files) != 0 || len(c.Symbols) != 0 {
		t.Fatalf("expected no changes, got %+v", c)
	}
}

func TestBuild_MapsHunksToSymbolsCallersAndTests(t *testing.T) {
	dir := repo(t)
	write(t, dir, "store/store.go", "package store\n\nfunc Open(name string) error {\n\tif name == \"\" {\n\t\treturn nil\n\t}\n\treturn nil\n}\n\nfunc Close() {}\n")
	write(t, dir, "store/extra.go", "package store\n\nfunc Extra() {}\n")
	write(t, dir, "README.md", "hi\n")

	c, err := Build(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]File{}
	for _, f := range c.Files {
		byPath[f.Path] = f
	}
	if byPath["store/store.go"].Status != "modified" || byPath["store/extra.go"].Status != "untracked" || byPath["README.md"].Status != "untracked" {
		t.Fatalf("files: %+v", c.Files)
	}
	var names []string
	for _, s := range c.Symbols {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Extra,Open" {
		t.Fatalf("changed symbols: %v", names)
	}
	open := c.Symbols[1]
	if len(open.Callers) != 1 || open.Callers[0].Key != "main.main" {
		t.Fatalf("callers: %+v", open.Callers)
	}
	if len(open.Tests) != 1 || open.Tests[0] != "store/store_test.go" {
		t.Fatalf("tests: %v", open.Tests)
	}
	if len(c.Tests) != 1 {
		t.Fatalf("tests to run: %v", c.Tests)
	}
	out := RenderText(c)
	for _, want := range []string{"store.Open", "← main.main 🚀", "store/store_test.go", "untracked  store/extra.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestBuild_AgainstRef(t *testing.T) {
	dir := repo(t)
	write(t, dir, "store/store.go", "package store\n\nfunc Open(name string) error {\n\treturn nil\n}\n\nfunc Close() { println() }\n")
	git(t, dir, "commit", "-qam", "touch close")

	c, err := Build(Config{RootDir: dir, Ref: "HEAD~1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Symbols) != 1 || c.Symbols[0].Name != "Close" {
		t.Fatalf("expected Close, got %+v", c.Symbols)
	}
	if _, err := Build(Config{RootDir: dir, Ref: "nope"}); err == nil {
		t.Fatal("bad ref should error")
	}
}

func TestBuild_NotARepo(t *testing.T) {
	_, err := Build(Config{RootDir: t.TempDir()})
	if !errors.Is(err, gitfacts.ErrNotARepo) {
		t.Fatalf("got %v", err)
	}
}

func TestHunkParsing(t *testing.T) {
	m := hunkRe.FindStringSubmatch("@@ -3,0 +4,2 @@ func x")
	if m == nil || m[1] != "4" || m[2] != "2" {
		t.Fatalf("got %v", m)
	}
	if m := hunkRe.FindStringSubmatch("@@ -1 +1 @@"); m == nil || m[1] != "1" || m[2] != "" {
		t.Fatalf("got %v", m)
	}
}
