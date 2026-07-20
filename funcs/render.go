package funcs

import (
	"fmt"
	"sort"
	"strings"
)

// RenderText groups signatures by file in source order.
func RenderText(r *Result, withDoc bool) string {
	if len(r.Funcs) == 0 {
		return "No functions found."
	}

	var b strings.Builder
	byFile := map[string][]Func{}
	var order []string
	for _, f := range r.Funcs {
		if _, seen := byFile[f.File]; !seen {
			order = append(order, f.File)
		}
		byFile[f.File] = append(byFile[f.File], f)
	}
	sort.Strings(order)

	for i, file := range order {
		if i > 0 {
			b.WriteString("\n")
		}
		fns := byFile[file]
		fmt.Fprintf(&b, "%s  (package %s, %d)\n", file, fns[0].Package, len(fns))
		for _, f := range fns {
			fmt.Fprintf(&b, "  %4d  %s\n", f.Line, f.Signature())
			if withDoc && f.Doc != "" {
				fmt.Fprintf(&b, "        %s\n", f.Doc)
			}
		}
	}

	fmt.Fprintf(&b, "\n%d function(s) in %d file(s).\n", len(r.Funcs), len(order))
	return b.String()
}

// RenderGrep is one line per function, `file:line: signature`.
func RenderGrep(r *Result) string {
	var b strings.Builder
	for _, f := range r.Funcs {
		fmt.Fprintf(&b, "%s:%d: %s\n", f.File, f.Line, f.Signature())
	}
	return strings.TrimRight(b.String(), "\n")
}

// RenderMarkdown is a table of signatures, for pasting into docs or an agent file.
func RenderMarkdown(r *Result) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Functions in %s\n\n", r.Root)
	b.WriteString("| Location | Signature | Doc |\n|---|---|---|\n")
	for _, f := range r.Funcs {
		doc := strings.ReplaceAll(f.Doc, "|", "\\|")
		fmt.Fprintf(&b, "| `%s:%d` | `%s` | %s |\n", f.File, f.Line, f.Signature(), doc)
	}
	return b.String()
}
