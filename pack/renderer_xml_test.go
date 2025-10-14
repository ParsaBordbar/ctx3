package pack

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderXML_Structure_OrderAndRootFiles(t *testing.T) {
	root := &dirNode{Name: ".", Children: nil, Files: []string{".gitignore"}}
	an := &dirNode{Name: "analyzer", Files: []string{"analyzer/analyzer.go"}}
	cmd := &dirNode{Name: "cmd", Files: []string{"cmd/root.go"}}
	root.Children = []*dirNode{cmd, an} // unsorted to test sorting

	var buf bytes.Buffer
	renderXMLStructure(&buf, root, Config{})
	out := buf.String()

	idxAnalyzer := strings.Index(out, "analyzer/\n")
	idxCmd := strings.Index(out, "cmd/\n")
	idxGitignore := strings.Index(out, ".gitignore\n")
	if idxAnalyzer < 0 || idxCmd < 0 || idxGitignore < 0 {
		t.Fatalf("missing expected lines in structure:\n%s", out)
	}
	if !(idxAnalyzer < idxGitignore && idxCmd < idxGitignore) {
		t.Fatalf("expected directories before root files; got:\n%s", out)
	}

	if !strings.Contains(out, "analyzer/\n  analyzer.go\n") {
		t.Fatalf("expected analyzer file under analyzer/ with indent; got:\n%s", out)
	}
	if !strings.Contains(out, "cmd/\n  root.go\n") {
		t.Fatalf("expected root.go under cmd/ with indent; got:\n%s", out)
	}

	if !strings.HasPrefix(out, "<directory_structure>\n") || !strings.Contains(out, "</directory_structure>\n\n") {
		t.Fatalf("missing directory_structure wrapper with default spacing:\n%s", out)
	}
}

func TestRenderXML_Files_NoCDATA_Deterministic(t *testing.T) {
	files := []FileEntry{
		{RelPath: "b/b.txt", Content: []byte("B")},
		{RelPath: "a/a.txt", Content: []byte("A")},
	}
	var buf bytes.Buffer
	renderXMLFiles(&buf, files, Config{})

	out := buf.String()
	if !strings.HasPrefix(out, "<files>\nThis section contains the contents of the repository's files.\n\n") {
		t.Fatalf("files header missing or spacing wrong:\n%s", out)
	}
	firstA := strings.Index(out, "<file path=\"a/a.txt\">")
	firstB := strings.Index(out, "<file path=\"b/b.txt\">")
	if !(firstA >= 0 && firstB > firstA) {
		t.Fatalf("expected a/a.txt before b/b.txt; got:\n%s", out)
	}
	// content present as-is, with exactly one newline before closing
	if !strings.Contains(out, "<file path=\"a/a.txt\">\nA\n</file>\n\n") {
		t.Fatalf("expected raw content for a/a.txt; got:\n%s", out)
	}
	if !strings.Contains(out, "<file path=\"b/b.txt\">\nB\n</file>\n\n") {
		t.Fatalf("expected raw content for b/b.txt; got:\n%s", out)
	}
	if !strings.HasSuffix(out, "</files>\n") {
		t.Fatalf("expected closing </files> tag; got:\n%s", out)
	}
}

func TestRenderXML_Compact_RemovesExtraBlankLines(t *testing.T) {
	files := []FileEntry{
		{RelPath: "a.txt", Content: []byte("A\n")},
		{RelPath: "b.txt", Content: []byte("B\n")},
	}
	root := &dirNode{Name: ".", Files: []string{"top.txt"}}
	var sbuf bytes.Buffer
	renderXMLStructure(&sbuf, root, Config{Compact: true})
	structOut := sbuf.String()
	// No extra blank line after closing tag
	if strings.Contains(structOut, "</directory_structure>\n\n") {
		t.Fatalf("compact mode should not add blank line after directory_structure, got:\n%s", structOut)
	}

	var fbuf bytes.Buffer
	renderXMLFiles(&fbuf, files, Config{Compact: true})
	filesOut := fbuf.String()

	if strings.Contains(filesOut, "files.\n\n") {
		t.Fatalf("compact mode should not add blank line after files header, got:\n%s", filesOut)
	}
	// No blank line between file blocks
	if strings.Contains(filesOut, "</file>\n\n<file path=") {
		t.Fatalf("compact mode should not add blank line between file blocks, got:\n%s", filesOut)
	}
	// Still ends properly
	if !strings.HasSuffix(filesOut, "</files>\n") {
		t.Fatalf("expected closing </files> in compact mode; got:\n%s", filesOut)
	}
}
