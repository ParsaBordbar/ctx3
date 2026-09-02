package diffctx

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/internal/tokens"
)

func RenderText(c *Context) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Change context — working tree vs %s\n", c.Ref)
	if c.Degraded {
		sb.WriteString("  (call graph is parse-only: the module does not type-check, caller lists may be incomplete)\n")
	}
	sb.WriteString("\n")

	if len(c.Files) == 0 {
		sb.WriteString("  No changes.\n")
	} else {
		fmt.Fprintf(&sb, "Files (%d):\n", len(c.Files))
		for _, f := range c.Files {
			fmt.Fprintf(&sb, "  %-10s %s  (+%d -%d)\n", f.Status, f.Path, f.Added, f.Removed)
		}
	}

	if len(c.Symbols) > 0 {
		fmt.Fprintf(&sb, "\nChanged symbols (%d):\n", len(c.Symbols))
		for _, s := range c.Symbols {
			var hs []string
			for _, h := range s.Hunks {
				if h.Start == h.End {
					hs = append(hs, fmt.Sprintf("%d", h.Start))
				} else {
					hs = append(hs, fmt.Sprintf("%d-%d", h.Start, h.End))
				}
			}
			fmt.Fprintf(&sb, "  %s.%s  (%s, %s:%d, lines %s)\n", s.Package, s.Name, s.Kind, s.File, s.Line, strings.Join(hs, ","))
			fmt.Fprintf(&sb, "      %s\n", s.Signature)
			for _, cl := range s.Callers {
				mark := ""
				if cl.IsEntry {
					mark = " 🚀"
				}
				fmt.Fprintf(&sb, "      ← %s%s  (%s:%d)\n", cl.Key, mark, cl.File, cl.Line)
			}
			if len(s.Entries) > 0 {
				fmt.Fprintf(&sb, "      reaches: %s\n", strings.Join(s.Entries, ", "))
			}
			if len(s.Tests) > 0 {
				fmt.Fprintf(&sb, "      tests:   %s\n", strings.Join(s.Tests, ", "))
			}
		}
	}

	if len(c.Tests) > 0 {
		fmt.Fprintf(&sb, "\nTests to run (%d):\n", len(c.Tests))
		for _, t := range c.Tests {
			fmt.Fprintf(&sb, "  %s\n", t)
		}
	}

	sb.WriteString("\n")
	for _, n := range c.Notes {
		fmt.Fprintf(&sb, "note: %s\n", n)
	}
	if c.Truncated {
		fmt.Fprintf(&sb, "trimmed to fit %s tokens\n", tokens.Format(c.Budget))
	}
	return sb.String()
}
