package funcs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const sample = `package sample

import "context"

// Add sums two ints.
// Second line is dropped.
func Add(a, b int) int { return a + b }

type Store struct{}

func (s *Store) Get(ctx context.Context, id string) (string, error) { return "", nil }

func Map[T any, R any](in []T, f func(T) R) []R { return nil }

func hidden() {}

func multi() (n int, err error) { return }

func none() {}
`

func writeSample(t *testing.T, name, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func scanSample(t *testing.T, cfg Config) map[string]Func {
	t.Helper()
	res, err := Scan(cfg)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	byName := map[string]Func{}
	for _, f := range res.Funcs {
		byName[f.Name] = f
	}
	return byName
}

func TestScan_Signatures(t *testing.T) {
	path := writeSample(t, "sample.go", sample)
	got := scanSample(t, Config{Path: path})

	want := map[string]string{
		"Add":    "func Add(a int, b int) int",
		"Get":    "func (s *Store) Get(ctx context.Context, id string) (string, error)",
		"Map":    "func Map[T any, R any](in []T, f func(T) R) []R",
		"hidden": "func hidden()",
		"multi":  "func multi() (n int, err error)",
		"none":   "func none()",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d functions, want %d", len(got), len(want))
	}
	for name, sig := range want {
		f, ok := got[name]
		if !ok {
			t.Errorf("missing function %q", name)
			continue
		}
		if f.Signature() != sig {
			t.Errorf("%s signature =\n  %q\nwant\n  %q", name, f.Signature(), sig)
		}
	}
}

func TestScan_DocIsFirstLineOnly(t *testing.T) {
	path := writeSample(t, "sample.go", sample)
	got := scanSample(t, Config{Path: path})
	if doc := got["Add"].Doc; doc != "Add sums two ints." {
		t.Errorf("Add doc = %q", doc)
	}
	if doc := got["hidden"].Doc; doc != "" {
		t.Errorf("hidden doc = %q, want empty", doc)
	}
}

func TestScan_ExportedOnlyAndMatch(t *testing.T) {
	path := writeSample(t, "sample.go", sample)

	exported := scanSample(t, Config{Path: path, ExportedOnly: true})
	for name := range exported {
		if strings.ToUpper(name[:1]) != name[:1] {
			t.Errorf("unexported %q survived ExportedOnly", name)
		}
	}
	if _, ok := exported["Get"]; !ok {
		t.Error("exported method Get was dropped")
	}

	matched := scanSample(t, Config{Path: path, Match: regexp.MustCompile("^M")})
	// Case-sensitive: Map matches, multi does not.
	if len(matched) != 1 {
		t.Fatalf("Match ^M returned %d functions: %v", len(matched), matched)
	}
	if _, ok := matched["Map"]; !ok {
		t.Errorf("Match ^M did not return Map, got %v", matched)
	}
}

func TestScan_TestFilesExcludedByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package p\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a_test.go"), []byte("package p\nfunc TestA() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanSample(t, Config{Path: dir}); len(got) != 1 || got["A"].Name != "A" {
		t.Errorf("default scan = %v, want only A", got)
	}
	got := scanSample(t, Config{Path: dir, IncludeTests: true})
	if len(got) != 2 {
		t.Fatalf("IncludeTests scan = %v, want 2", got)
	}
	if !got["TestA"].Test {
		t.Error("TestA not flagged as Test")
	}
}

func TestScan_RecursiveOptIn(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.go"), []byte("package p\nfunc Top() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep.go"), []byte("package q\nfunc Deep() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := scanSample(t, Config{Path: dir}); len(got) != 1 {
		t.Errorf("non-recursive scan = %v, want only Top", got)
	}
	if got := scanSample(t, Config{Path: dir, Recursive: true}); len(got) != 2 {
		t.Errorf("recursive scan = %v, want Top and Deep", got)
	}
}

func TestScan_UnparseableFileDoesNotSinkScan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.go"), []byte("package p\nfunc Ok() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package p\nfunc Broken( {"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := scanSample(t, Config{Path: dir}); len(got) != 1 || got["Ok"].Name != "Ok" {
		t.Errorf("scan with broken file = %v, want only Ok", got)
	}
}

func TestRenderGrep_IsFileLineSignature(t *testing.T) {
	path := writeSample(t, "sample.go", sample)
	res, err := Scan(Config{Path: path, Match: regexp.MustCompile("^Add$")})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.ToSlash(path) + ":7: func Add(a int, b int) int"
	if got := RenderGrep(res); got != want {
		t.Errorf("RenderGrep = %q, want %q", got, want)
	}
}
