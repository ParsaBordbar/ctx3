package pack

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// txtRule separates sections. Plain text has no markup to delimit a file, so a
// visually distinct banner is what tells a reader — and a model — where one
// file's content ends and the next begins.
var txtRule = strings.Repeat("=", 64)

func renderTXTStructure(buf *bytes.Buffer, tree *dirNode, cfg Config) {
	fmt.Fprintf(buf, "%s\nDirectory structure\n%s\n", txtRule, txtRule)
	if tree != nil {
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
		sort.Strings(rootFiles)
		for _, rf := range rootFiles {
			buf.WriteString(base(rf))
			buf.WriteByte('\n')
		}
	}
	if !cfg.Compact {
		buf.WriteByte('\n')
	}
}

func renderTXTFiles(buf *bytes.Buffer, files []FileEntry, cfg Config) {
	fmt.Fprintf(buf, "%s\nFiles\n%s\n", txtRule, txtRule)
	if !cfg.Compact {
		buf.WriteByte('\n')
	}

	sorted := make([]FileEntry, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RelPath < sorted[j].RelPath })

	for _, f := range sorted {
		fmt.Fprintf(buf, "%s\nFile: %s\n%s\n", txtRule, f.RelPath, txtRule)
		if len(f.Content) > 0 {
			buf.Write(f.Content)
			if f.Content[len(f.Content)-1] != '\n' {
				buf.WriteByte('\n')
			}
		}
		if !cfg.Compact {
			buf.WriteByte('\n')
		}
	}
}
