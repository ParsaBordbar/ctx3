package brief

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/internal/style"
	"github.com/parsabordbar/ctx3/internal/tokens"
)

func RenderText(b *Brief) string { return RenderStyled(b, style.Plain) }

func RenderStyled(b *Brief, st style.Palette) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", st.Title("Brief —"), st.Bold(fmt.Sprintf("%q", b.Query)))
	if b.Degraded {
		sb.WriteString(st.Warn("  (call graph is parse-only: the module does not type-check, caller lists may be incomplete)") + "\n")
	}
	sb.WriteString("\n")
	if len(b.Hits) == 0 {
		sb.WriteString(st.Dim("  No matching symbols.") + "\n")
	}
	for i, h := range b.Hits {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s %s  %s\n", st.Accent("##"), st.Name(displayName(h)),
			st.Dim(fmt.Sprintf("(%s, %s:%d)", h.Kind, h.File, h.Line)))
		if h.Doc != "" {
			fmt.Fprintf(&sb, "   %s\n", st.Dim(h.Doc))
		}
		if h.Source != "" {
			sb.WriteString(st.Dim("```") + "\n")
			sb.WriteString(h.Source)
			sb.WriteString("\n" + st.Dim("```") + "\n")
		} else {
			fmt.Fprintf(&sb, "   %s\n", h.Signature)
		}
		if len(h.Calls) > 0 {
			fmt.Fprintf(&sb, "   %s %s\n", st.Dim("calls:   "), strings.Join(h.Calls, ", "))
		}
		if len(h.Callers) > 0 {
			sb.WriteString("   " + st.Dim("callers:") + "\n")
			for _, c := range h.Callers {
				mark := ""
				if c.IsEntry {
					mark = " 🚀"
				}
				fmt.Fprintf(&sb, "     %s%s  %s\n", st.Accent(c.Key), mark,
					st.Dim(fmt.Sprintf("(%s:%d, depth %d)", c.File, c.Line, c.Depth)))
			}
		} else if h.Kind == "func" || h.Kind == "method" {
			if len(h.Entries) == 0 && h.Source != "" && !b.Degraded && h.Callers == nil && len(h.Calls) == 0 {
				sb.WriteString("   " + st.Dim("callers:  none found") + "\n")
			}
		}
		if len(h.Entries) > 0 {
			fmt.Fprintf(&sb, "   %s %s\n", st.Dim("reaches: "), st.Ok(strings.Join(h.Entries, ", ")))
		}
		if len(h.Imports) > 0 {
			fmt.Fprintf(&sb, "   %s %s\n", st.Dim("package imports:"), strings.Join(h.Imports, ", "))
		}
	}
	sb.WriteString("\n")
	for _, n := range b.Notes {
		fmt.Fprintf(&sb, "%s %s\n", st.Warn("note:"), n)
	}
	if b.Truncated {
		sb.WriteString(st.Warn(fmt.Sprintf("trimmed to fit %s tokens", tokens.Format(b.Budget))) + "\n")
	}
	return sb.String()
}

func displayName(h Hit) string {
	if h.Package != "" {
		return h.Package + "." + h.Name
	}
	return h.Name
}
