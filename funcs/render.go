package funcs

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"sort"
	"strings"
)

// RenderText groups signatures by file in source order.
func RenderText(r *Result, withDoc bool) string { return RenderStyled(r, withDoc, style.Plain) }

func RenderStyled(r *Result, withDoc bool, st style.Palette) string {
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
		fmt.Fprintf(&b, "%s  %s\n", st.Title(file), st.Dim(fmt.Sprintf("(package %s, %d)", fns[0].Package, len(fns))))
		for _, f := range fns {
			sig := f.Signature()
			if st.Enabled() {
				if i := strings.Index(sig, f.Name+"("); i >= 0 {
					sig = sig[:i] + st.Name(f.Name) + sig[i+len(f.Name):]
				}
			}
			fmt.Fprintf(&b, "  %s  %s\n", st.Dim(fmt.Sprintf("%4d", f.Line)), sig)
			if withDoc && f.Doc != "" {
				fmt.Fprintf(&b, "        %s\n", st.Dim(f.Doc))
			}
		}
	}

	b.WriteString(st.Dim(fmt.Sprintf("\n%d function(s) in %d file(s).", len(r.Funcs), len(order))) + "\n")
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
