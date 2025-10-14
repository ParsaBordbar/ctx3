package pack

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// renderXMLStructure writes:
// <directory_structure>
// analyzer/
//
//	analyzer.go
//
// ...
// </directory_structure>
func renderXMLStructure(buf *bytes.Buffer, tree *dirNode, cfg Config) {
	buf.WriteString("<directory_structure>\n")
	if tree != nil {
		// render child directories first (sorted), then root files (sorted)
		children := make([]*dirNode, len(tree.Children))
		copy(children, tree.Children)
		sort.Slice(children, func(i, j int) bool {
			return base(children[i].Name) < base(children[j].Name)
		})
		for _, ch := range children {
			renderDirNode(buf, ch, 0)
		}

		rootFiles := make([]string, len(tree.Files))
		copy(rootFiles, tree.Files)
		sort.Slice(rootFiles, func(i, j int) bool { return rootFiles[i] < rootFiles[j] })
		for _, rf := range rootFiles {
			buf.WriteString(base(rf))
			buf.WriteByte('\n')
		}
	}
	if cfg.Compact {
		buf.WriteString("</directory_structure>\n")
	} else {
		buf.WriteString("</directory_structure>\n\n")
	}
}

func renderDirNode(buf *bytes.Buffer, n *dirNode, depth int) {
	indent := strings.Repeat("  ", depth)
	buf.WriteString(indent)
	buf.WriteString(base(n.Name))
	buf.WriteString("/\n")

	// files in this directory (basenames), sorted
	files := make([]string, len(n.Files))
	copy(files, n.Files)
	sort.Slice(files, func(i, j int) bool { return files[i] < files[j] })
	for _, f := range files {
		buf.WriteString(indent)
		buf.WriteString("  ")
		buf.WriteString(base(f))
		buf.WriteByte('\n')
	}

	// child directories (sorted by basename)
	children := make([]*dirNode, len(n.Children))
	copy(children, n.Children)
	sort.Slice(children, func(i, j int) bool { return base(children[i].Name) < base(children[j].Name) })
	for _, ch := range children {
		renderDirNode(buf, ch, depth+1)
	}
}

// renderXMLFiles writes (no CDATA; matches your sample).
// If cfg.Compact is true, it removes the extra blank lines between file blocks
// and after the section header.
func renderXMLFiles(buf *bytes.Buffer, files []FileEntry, cfg Config) {
	buf.WriteString("<files>\n")
	if cfg.Compact {
		buf.WriteString("This section contains the contents of the repository's files.\n")
	} else {
		buf.WriteString("This section contains the contents of the repository's files.\n\n")
	}

	// deterministic order by RelPath
	sorted := make([]FileEntry, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RelPath < sorted[j].RelPath })

	for _, f := range sorted {
		fmt.Fprintf(buf, "<file path=\"%s\">\n", f.RelPath)
		if len(f.Content) > 0 {
			buf.Write(f.Content)
			// ensure exactly one trailing newline before </file>
			if f.Content[len(f.Content)-1] != '\n' {
				buf.WriteByte('\n')
			}
		} else {
			// keep a blank line for empty files to match layout
			buf.WriteByte('\n')
		}
		buf.WriteString("</file>\n")
		// In non-compact mode, always add a blank line after every </file>,
		// including the last one, to match the sample output and tests.
		if !cfg.Compact {
			buf.WriteByte('\n')
		}
	}

	buf.WriteString("</files>\n")
}

// base returns the last path element using '/' separator semantics.
func base(rel string) string {
	if rel == "." || rel == "" {
		return "."
	}
	r := strings.TrimSuffix(rel, "/")
	if i := strings.LastIndexByte(r, '/'); i >= 0 {
		return r[i+1:]
	}
	return r
}
