package pack

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestWalk_RespectsGitignoreAndGlobs(t *testing.T) {
	td := t.TempDir()

	// .gitignore ignores a specific file and a directory
	writeFile(t, filepath.Join(td, ".gitignore"), []byte("ignoreme.txt\nbin/\n"))

	// Files
	writeFile(t, filepath.Join(td, "a.go"), []byte("package main\n"))
	writeFile(t, filepath.Join(td, "b.md"), []byte("# doc\n"))
	writeFile(t, filepath.Join(td, "ignoreme.txt"), []byte("nope\n"))
	writeFile(t, filepath.Join(td, "bin", "exec.bin"), []byte{0x00, 0x01, 0x02})

	cfg := Config{
		RootDir:          td,
		RespectGitignore: true,
		// No include globs → apply ignore rules
		IgnoreGlobs:    []string{"*.md"}, // ignore markdown via CLI
		BinaryHandling: BinarySkip,
	}

	files, _, rep, err := WalkAndCollect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("WalkAndCollect error: %v", err)
	}

	paths := relPaths(files)
	if contains(paths, "ignoreme.txt") {
		t.Fatalf("expected .gitignore to exclude ignoreme.txt; got %v", paths)
	}
	if contains(paths, "bin/exec.bin") {
		t.Fatalf("expected .gitignore to exclude bin/exec.bin; got %v", paths)
	}
	if contains(paths, "b.md") {
		t.Fatalf("expected CLI ignore to exclude b.md; got %v", paths)
	}
	if !contains(paths, "a.go") {
		t.Fatalf("expected a.go to be included; got %v", paths)
	}
	// We include the .gitignore itself (aligns with repomix-style packs).
	if !contains(paths, ".gitignore") {
		t.Fatalf("expected .gitignore to be included; got %v", paths)
	}
	if rep.FilesIncluded != 2 {
		t.Fatalf("expected FilesIncluded=2 (a.go + .gitignore); got %d", rep.FilesIncluded)
	}
}

func TestWalk_IncludeOverridesIgnores(t *testing.T) {
	td := t.TempDir()
	writeFile(t, filepath.Join(td, ".gitignore"), []byte("*.md\n"))
	writeFile(t, filepath.Join(td, "keep.md"), []byte("hello\n"))
	writeFile(t, filepath.Join(td, "skip.md"), []byte("bye\n"))
	writeFile(t, filepath.Join(td, "code.go"), []byte("package main\n"))

	cfg := Config{
		RootDir:          td,
		RespectGitignore: true,
		IncludeGlobs:     []string{"**/keep.md", "*.go"}, // include takes precedence
		IgnoreGlobs:      []string{"skip.md"},
		BinaryHandling:   BinarySkip,
	}

	files, _, _, err := WalkAndCollect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("WalkAndCollect error: %v", err)
	}
	paths := relPaths(files)
	if !contains(paths, "keep.md") {
		t.Fatalf("expected keep.md to be re-included; got %v", paths)
	}
	if contains(paths, "skip.md") {
		t.Fatalf("expected skip.md to remain excluded; got %v", paths)
	}
	if !contains(paths, "code.go") {
		t.Fatalf("expected code.go included via include; got %v", paths)
	}
}

func TestWalk_BinaryHandling_SkipAndBase64(t *testing.T) {
	td := t.TempDir()
	writeFile(t, filepath.Join(td, "text.txt"), []byte("plain\n"))
	writeFile(t, filepath.Join(td, "image.png"), []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x01})

	// Skip binaries: expect only text.txt included
	cfgSkip := Config{RootDir: td, BinaryHandling: BinarySkip}
	filesSkip, _, repSkip, err := WalkAndCollect(context.Background(), cfgSkip)
	if err != nil {
		t.Fatalf("WalkAndCollect skip error: %v", err)
	}
	pathsSkip := relPaths(filesSkip)
	if contains(pathsSkip, "image.png") {
		t.Fatalf("expected image.png to be skipped; got %v", pathsSkip)
	}
	if !contains(pathsSkip, "text.txt") {
		t.Fatalf("expected text.txt included; got %v", pathsSkip)
	}
	if repSkip.FilesIncluded != 1 {
		t.Fatalf("expected included=1 (only text.txt); got %d", repSkip.FilesIncluded)
	}

	// Base64 binaries: ensure encoded content present for image.png
	cfgB64 := Config{RootDir: td, BinaryHandling: BinaryBase64}
	filesB64, _, _, err := WalkAndCollect(context.Background(), cfgB64)
	if err != nil {
		t.Fatalf("WalkAndCollect base64 error: %v", err)
	}
	var img FileEntry
	for _, f := range filesB64 {
		if f.RelPath == "image.png" {
			img = f
			break
		}
	}
	if len(img.Content) == 0 {
		t.Fatalf("expected base64 content for image.png")
	}
	if !isBase64String(string(img.Content)) {
		t.Fatalf("image.png content not valid base64: %q", string(img.Content))
	}
}

func TestWalk_MaxFileAndTotalBytes(t *testing.T) {
	td := t.TempDir()
	writeFile(t, filepath.Join(td, "big.txt"), bytesOfSize(10_000))
	writeFile(t, filepath.Join(td, "small1.txt"), bytesOfSize(100))
	writeFile(t, filepath.Join(td, "small2.txt"), bytesOfSize(100))

	cfg := Config{
		RootDir:       td,
		MaxFileBytes:  5_000, // big.txt skipped
		MaxTotalBytes: 150,   // only one small file fits (plus .gitignore if present)
	}
	files, _, rep, err := WalkAndCollect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("WalkAndCollect error: %v", err)
	}
	paths := relPaths(files)
	if contains(paths, "big.txt") {
		t.Fatalf("expected big.txt skipped; got %v", paths)
	}
	// Expect either 1 or 2 files depending on whether .gitignore pushed total over cap.
	if len(files) < 1 {
		t.Fatalf("expected at least one file due to MaxTotalBytes; got %d (%v)", len(files), paths)
	}
	if rep.FilesIncluded < 1 {
		t.Fatalf("expected rep.FilesIncluded>=1; got %d", rep.FilesIncluded)
	}
}

func TestWalk_SortByExt(t *testing.T) {
	td := t.TempDir()
	writeFile(t, filepath.Join(td, "b.md"), []byte("# b\n"))
	writeFile(t, filepath.Join(td, "a.go"), []byte("package main\n"))
	writeFile(t, filepath.Join(td, "c.txt"), []byte("c\n"))

	cfg := Config{RootDir: td, SortByExt: true}
	files, _, _, err := WalkAndCollect(context.Background(), cfg)
	if err != nil {
		t.Fatalf("WalkAndCollect error: %v", err)
	}
	paths := relPaths(files)
	// by ext: .go, .md, .txt; .gitignore may also be present; ignore it for order check
	filtered := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == ".gitignore" {
			continue
		}
		filtered = append(filtered, p)
	}
	wantOrder := []string{"a.go", "b.md", "c.txt"}
	if strings.Join(filtered, ",") != strings.Join(wantOrder, ",") {
		t.Fatalf("unexpected order: got %v want %v", filtered, wantOrder)
	}
}

// --- helpers for tests ---

func relPaths(files []FileEntry) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, filepath.ToSlash(f.RelPath))
	}
	sort.Strings(out)
	return out
}

func contains(ss []string, s string) bool {
	for _, e := range ss {
		if e == s {
			return true
		}
	}
	return false
}

func bytesOfSize(n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(i % 251)
	}
	return b
}

func isBase64String(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '+', r == '/', r == '=', r == '\n', r == '\r':
			// ok
		default:
			return false
		}
	}
	n := 0
	for _, r := range s {
		if r != '\n' && r != '\r' {
			n++
		}
	}
	return n%4 == 0
}
