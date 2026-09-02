package symbols

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"sort"
	"strings"
)

// RenderText groups symbols by directory, then by kind — the skimmable default.
func RenderText(idx *Index, docs bool) string { return RenderStyled(idx, docs, style.Plain) }

func RenderStyled(idx *Index, docs bool, st style.Palette) string {
	if len(idx.Symbols) == 0 {
		return "No symbols found."
	}

	byDir := map[string][]Symbol{}
	for _, s := range idx.Symbols {
		byDir[s.Dir] = append(byDir[s.Dir], s)
	}

	var sb strings.Builder
	for _, dir := range idx.Packages() {
		syms := byDir[dir]
		fmt.Fprintf(&sb, "%s  %s\n", st.Title(dir), st.Dim(fmt.Sprintf("(%s, %d symbols)", syms[0].Package, len(syms))))
		lastKind := Kind("")
		for _, s := range syms {
			if s.Kind != lastKind {
				fmt.Fprintf(&sb, "  %s\n", st.Keyword(kindHeading(s.Kind)))
				lastKind = s.Kind
			}
			fmt.Fprintf(&sb, "    %s %s\n", style.Ljust(highlightName(st, truncate(s.Signature, 52), s.Name), 52),
				st.Dim(fmt.Sprintf("%s:%d", s.File, s.Line)))
			if docs && s.Doc != "" {
				fmt.Fprintf(&sb, "    %s%s\n", strings.Repeat(" ", 2), st.Dim(s.Doc))
			}
			for _, m := range s.Members {
				fmt.Fprintf(&sb, "      %s %s %s\n", st.Dim("·"), m.Name, st.Dim(m.Type))
			}
		}
		sb.WriteByte('\n')
	}

	sb.WriteString(st.Dim(fmt.Sprintf("%d symbols across %d packages (%s)",
		len(idx.Symbols), len(byDir), strings.Join(Counts(idx), ", "))) + "\n")
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

// RenderGrep is one `file:line: signature` per symbol, for pipes and editors.
func RenderGrep(idx *Index) string {
	var sb strings.Builder
	for _, s := range idx.Symbols {
		fmt.Fprintf(&sb, "%s:%d: %s\n", s.File, s.Line, s.Signature)
	}
	return sb.String()
}

// RenderMarkdown is a per-package table, for pasting into an agent context file.
func RenderMarkdown(idx *Index) string {
	byDir := map[string][]Symbol{}
	for _, s := range idx.Symbols {
		byDir[s.Dir] = append(byDir[s.Dir], s)
	}

	var sb strings.Builder
	sb.WriteString("# Symbol map\n")
	for _, dir := range idx.Packages() {
		fmt.Fprintf(&sb, "\n## %s\n\n", dir)
		sb.WriteString("| Kind | Symbol | Location | Doc |\n|---|---|---|---|\n")
		for _, s := range byDir[dir] {
			fmt.Fprintf(&sb, "| %s | `%s` | %s:%d | %s |\n",
				s.Kind, s.Signature, s.File, s.Line, s.Doc)
		}
	}
	return sb.String()
}

// Counts returns how many symbols of each kind the index holds, kind-sorted.
func Counts(idx *Index) []string {
	n := map[Kind]int{}
	for _, s := range idx.Symbols {
		n[s.Kind]++
	}
	kinds := make([]Kind, 0, len(n))
	for k := range n {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return kindOrder[kinds[i]] < kindOrder[kinds[j]] })

	out := make([]string, 0, len(kinds))
	for _, k := range kinds {
		out = append(out, fmt.Sprintf("%d %s", n[k], k))
	}
	return out
}

func kindHeading(k Kind) string {
	switch k {
	case KindStruct:
		return "structs"
	case KindInterface:
		return "interfaces"
	case KindMethod:
		return "methods"
	case KindType:
		return "types"
	case KindConst:
		return "consts"
	case KindVar:
		return "vars"
	}
	return "funcs"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func highlightName(st style.Palette, sig, name string) string {
	if !st.Enabled() || name == "" {
		return sig
	}
	i := strings.Index(sig, name)
	if i < 0 {
		return sig
	}
	return sig[:i] + st.Name(name) + sig[i+len(name):]
}
