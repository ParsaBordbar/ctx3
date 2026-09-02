package pack

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

// renderMDStructure writes the directory tree inside a fenced block, so the
// whole document stays valid Markdown when pasted into a chat or an issue.
func renderMDStructure(buf *bytes.Buffer, tree *dirNode, cfg Config) {
	buf.WriteString("# Directory structure\n\n")
	buf.WriteString("```\n")
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
	buf.WriteString("```\n")
	if !cfg.Compact {
		buf.WriteByte('\n')
	}
}

// renderMDFiles writes one section per file, each body in a fenced block tagged
// with the language so a reader (and a syntax highlighter) can tell what it is.
func renderMDFiles(buf *bytes.Buffer, files []FileEntry, cfg Config) {
	buf.WriteString("# Files\n")
	if !cfg.Compact {
		buf.WriteByte('\n')
	}

	sorted := make([]FileEntry, len(files))
	copy(sorted, files)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].RelPath < sorted[j].RelPath })

	for _, f := range sorted {
		fmt.Fprintf(buf, "## %s\n", f.RelPath)
		if !cfg.Compact {
			buf.WriteByte('\n')
		}

		fence := fenceFor(f.Content)
		buf.WriteString(fence)
		buf.WriteString(mdLang(f.RelPath))
		buf.WriteByte('\n')
		if len(f.Content) > 0 {
			buf.Write(f.Content)
			if f.Content[len(f.Content)-1] != '\n' {
				buf.WriteByte('\n')
			}
		}
		buf.WriteString(fence)
		buf.WriteByte('\n')
		if !cfg.Compact {
			buf.WriteByte('\n')
		}
	}
}

// fenceFor returns a fence longer than the longest backtick run in the content.
// A file that itself contains ``` would otherwise close the block early and
// corrupt every section after it.
func fenceFor(content []byte) string {
	longest, run := 0, 0
	for _, b := range content {
		if b == '`' {
			run++
			if run > longest {
				longest = run
			}
			continue
		}
		run = 0
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

// mdLangByExt maps a file extension to a Markdown fence language tag.
var mdLangByExt = map[string]string{
	"go": "go", "py": "python", "rb": "ruby", "rs": "rust", "java": "java",
	"ts": "typescript", "tsx": "tsx", "js": "javascript", "jsx": "jsx",
	"mjs": "javascript", "cjs": "javascript", "sh": "bash", "bash": "bash",
	"zsh": "bash", "json": "json", "yaml": "yaml", "yml": "yaml",
	"toml": "toml", "md": "markdown", "sql": "sql", "html": "html",
	"css": "css", "scss": "scss", "xml": "xml", "c": "c", "h": "c",
	"cpp": "cpp", "hpp": "cpp", "cs": "csharp", "php": "php", "kt": "kotlin",
	"swift": "swift", "proto": "protobuf", "dockerfile": "dockerfile",
}

// mdLang picks the fence tag for a path, or "" when nothing fits.
func mdLang(rel string) string {
	name := base(rel)
	if strings.EqualFold(name, "Dockerfile") {
		return "dockerfile"
	}
	if strings.EqualFold(name, "Makefile") {
		return "makefile"
	}
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return mdLangByExt[strings.ToLower(name[i+1:])]
}
