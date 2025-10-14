package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackCommand_XML_ToStdout(t *testing.T) {
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "x.txt"), []byte("hello\n"))

	// Capture stdout (pack currently writes with fmt.Print)
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	// Run: ctx3 pack <td> --format xml
	rootCmd.SetArgs([]string{"pack", td, "--format", "xml"})
	err = rootCmd.Execute()

	// Restore stdout
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, cerr := io.Copy(&buf, r); cerr != nil {
		t.Fatalf("read stdout: %v", cerr)
	}
	r.Close()

	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "<directory_structure>\n") {
		t.Fatalf("expected directory_structure header; got:\n%s", out)
	}
	if !strings.Contains(out, `<file path="x.txt">`) {
		t.Fatalf("expected files section to include x.txt; got:\n%s", out)
	}
}

func TestPackCommand_XML_ToFile(t *testing.T) {
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "y.txt"), []byte("world\n"))

	outFile := filepath.Join(td, "pack.xml")
	rootCmd.SetArgs([]string{"pack", td, "--format", "xml", "-o", outFile})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	b, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read packed file: %v", err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "<directory_structure>\n") || !strings.Contains(s, "<files>\n") {
		t.Fatalf("packed file missing sections:\n%s", s)
	}
}

func mustWrite(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
