package pack

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPack_XML_AllSections(t *testing.T) {
	td := t.TempDir()

	// Create tiny repo
	mustWrite(t, filepath.Join(td, ".gitignore"), []byte("tmp/\n"))
	mustWrite(t, filepath.Join(td, "analyzer", "analyzer.go"), []byte("package analyzer\n"))
	mustWrite(t, filepath.Join(td, "cmd", "root.go"), []byte("package cmd\n"))
	mustWrite(t, filepath.Join(td, "README.md"), []byte("# ctx3\n"))

	cfg := Config{
		RootDir:          td,
		OutputFormat:     FormatXML,
		RespectGitignore: true,
		BinaryHandling:   BinarySkip,
		Sections:         Sections{Structure: true, Files: true},
	}
	out, rep, err := Pack(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}
	if rep.FilesIncluded == 0 {
		t.Fatalf("expected at least 1 file included")
	}

	s := string(out)
	// Has directory_structure first and files after
	dsIdx := strings.Index(s, "<directory_structure>\n")
	fsIdx := strings.Index(s, "<files>\n")
	if !(dsIdx >= 0 && fsIdx > dsIdx) {
		t.Fatalf("expected <directory_structure> before <files>, got:\n%s", s)
	}

	// Structure contains directories with proper indentation
	if !strings.Contains(s, "analyzer/\n  analyzer.go\n") {
		t.Fatalf("missing analyzer entry in structure:\n%s", s)
	}
	if !strings.Contains(s, "cmd/\n  root.go\n") {
		t.Fatalf("missing cmd entry in structure:\n%s", s)
	}

	// Files section contains content blocks for created files (paths are relative)
	if !strings.Contains(s, "<file path=\"README.md\">\n# ctx3\n</file>\n\n") {
		t.Fatalf("missing README.md content block:\n%s", s)
	}
	if !strings.Contains(s, "<file path=\"analyzer/analyzer.go\">\npackage analyzer\n</file>\n\n") {
		t.Fatalf("missing analyzer/analyzer.go content block:\n%s", s)
	}
}

func TestPack_XML_StructureOnly(t *testing.T) {
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "a", "x.txt"), []byte("x"))
	mustWrite(t, filepath.Join(td, "b", "y.txt"), []byte("y"))

	cfg := Config{
		RootDir:      td,
		OutputFormat: FormatXML,
		Sections:     Sections{Structure: true, Files: false},
	}
	out, _, err := Pack(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "<directory_structure>\n") || !strings.Contains(s, "</directory_structure>\n\n") {
		t.Fatalf("expected only directory_structure section:\n%s", s)
	}
	if strings.Contains(s, "<files>") {
		t.Fatalf("did not expect <files> section:\n%s", s)
	}
	// Should list directories with files under them
	if !strings.Contains(s, "a/\n  x.txt\n") || !strings.Contains(s, "b/\n  y.txt\n") {
		t.Fatalf("structure listing incomplete:\n%s", s)
	}
}

func TestPack_XML_FilesOnly(t *testing.T) {
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "top.txt"), []byte("hi"))
	mustWrite(t, filepath.Join(td, "dir", "sub.txt"), []byte("there"))

	cfg := Config{
		RootDir:      td,
		OutputFormat: FormatXML,
		Sections:     Sections{Structure: false, Files: true},
	}
	out, _, err := Pack(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Pack error: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "<directory_structure>") {
		t.Fatalf("did not expect directory_structure section:\n%s", s)
	}
	if !strings.HasPrefix(s, "<files>\nThis section contains the contents of the repository's files.\n\n") {
		t.Fatalf("expected <files> section with header:\n%s", s)
	}
	if !strings.Contains(s, "<file path=\"dir/sub.txt\">\nthere\n</file>\n\n") {
		t.Fatalf("missing dir/sub.txt content block:\n%s", s)
	}
	if !strings.Contains(s, "<file path=\"top.txt\">\nhi\n</file>\n\n") {
		t.Fatalf("missing top.txt content block:\n%s", s)
	}
}

func TestPack_UnsupportedFormat(t *testing.T) {
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "f.txt"), []byte("z"))
	cfg := Config{
		RootDir:      td,
		OutputFormat: FormatMD, // TODO: not implemented yet
		Sections:     Sections{Structure: true, Files: true},
	}
	_, _, err := Pack(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got: %v", err)
	}
}

// --- helpers ---

func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
