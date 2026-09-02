package diffctx

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/internal/style"
	"github.com/parsabordbar/ctx3/internal/tokens"
)

func RenderText(c *Context) string { return RenderStyled(c, style.Plain) }

func statusColor(st style.Palette, status string) string {
	switch status {
	case "added", "untracked":
		return st.Ok(status)
	case "deleted":
		return st.Err(status)
	default:
		return st.Warn(status)
	}
}

func RenderStyled(c *Context, st style.Palette) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s %s\n", st.Title("Change context —"), st.Bold("working tree vs "+c.Ref))
	if c.Degraded {
		sb.WriteString(st.Warn("  (call graph is parse-only: the module does not type-check, caller lists may be incomplete)") + "\n")
	}
	sb.WriteString("\n")

	if len(c.Files) == 0 {
		sb.WriteString(st.Dim("  No changes.") + "\n")
	} else {
		fmt.Fprintf(&sb, "%s\n", st.Bold(fmt.Sprintf("Files (%d):", len(c.Files))))
		for _, f := range c.Files {
			fmt.Fprintf(&sb, "  %s %s  %s\n", style.Ljust(statusColor(st, f.Status), 10), f.Path,
				st.Dim(fmt.Sprintf("(+%d -%d)", f.Added, f.Removed)))
		}
	}

	if len(c.Symbols) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", st.Bold(fmt.Sprintf("Changed symbols (%d):", len(c.Symbols))))
		for _, s := range c.Symbols {
			var hs []string
			for _, h := range s.Hunks {
				if h.Start == h.End {
					hs = append(hs, fmt.Sprintf("%d", h.Start))
				} else {
					hs = append(hs, fmt.Sprintf("%d-%d", h.Start, h.End))
				}
			}
			fmt.Fprintf(&sb, "  %s  %s\n", st.Name(s.Package+"."+s.Name),
				st.Dim(fmt.Sprintf("(%s, %s:%d, lines %s)", s.Kind, s.File, s.Line, strings.Join(hs, ","))))
			fmt.Fprintf(&sb, "      %s\n", s.Signature)
			for _, cl := range s.Callers {
				mark := ""
				if cl.IsEntry {
					mark = st.Glyph(" 🚀", " [entry]")
				}
				fmt.Fprintf(&sb, "      %s %s%s  %s\n", st.Dim("←"), st.Accent(cl.Key), mark,
					st.Dim(fmt.Sprintf("(%s:%d)", cl.File, cl.Line)))
			}
			if len(s.Entries) > 0 {
				fmt.Fprintf(&sb, "      %s %s\n", st.Dim("reaches:"), st.Ok(strings.Join(s.Entries, ", ")))
			}
			if len(s.Tests) > 0 {
				fmt.Fprintf(&sb, "      %s   %s\n", st.Dim("tests:"), st.Path(strings.Join(s.Tests, ", ")))
			}
		}
	}

	if len(c.Tests) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", st.Bold(fmt.Sprintf("Tests to run (%d):", len(c.Tests))))
		for _, t := range c.Tests {
			fmt.Fprintf(&sb, "  %s\n", st.Path(t))
		}
	}

	sb.WriteString("\n")
	for _, n := range c.Notes {
		fmt.Fprintf(&sb, "%s %s\n", st.Warn("note:"), n)
	}
	if c.Truncated {
		sb.WriteString(st.Warn(fmt.Sprintf("trimmed to fit %s tokens", tokens.Format(c.Budget))) + "\n")
	}
	return sb.String()
}
