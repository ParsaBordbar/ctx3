package brief

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/internal/tokens"
)

func RenderText(b *Brief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Brief — %q\n", b.Query)
	if b.Degraded {
		sb.WriteString("  (call graph is parse-only: the module does not type-check, caller lists may be incomplete)\n")
	}
	sb.WriteString("\n")
	if len(b.Hits) == 0 {
		sb.WriteString("  No matching symbols.\n")
	}
	for i, h := range b.Hits {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "## %s  (%s, %s:%d)\n", displayName(h), h.Kind, h.File, h.Line)
		if h.Doc != "" {
			fmt.Fprintf(&sb, "   %s\n", h.Doc)
		}
		if h.Source != "" {
			sb.WriteString("```\n")
			sb.WriteString(h.Source)
			sb.WriteString("\n```\n")
		} else {
			fmt.Fprintf(&sb, "   %s\n", h.Signature)
		}
		if len(h.Calls) > 0 {
			fmt.Fprintf(&sb, "   calls:    %s\n", strings.Join(h.Calls, ", "))
		}
		if len(h.Callers) > 0 {
			sb.WriteString("   callers:\n")
			for _, c := range h.Callers {
				mark := ""
				if c.IsEntry {
					mark = " 🚀"
				}
				fmt.Fprintf(&sb, "     %s%s  (%s:%d, depth %d)\n", c.Key, mark, c.File, c.Line, c.Depth)
			}
		} else if h.Kind == "func" || h.Kind == "method" {
			if len(h.Entries) == 0 && h.Source != "" && !b.Degraded && h.Callers == nil && len(h.Calls) == 0 {
				sb.WriteString("   callers:  none found\n")
			}
		}
		if len(h.Entries) > 0 {
			fmt.Fprintf(&sb, "   reaches:  %s\n", strings.Join(h.Entries, ", "))
		}
		if len(h.Imports) > 0 {
			fmt.Fprintf(&sb, "   package imports: %s\n", strings.Join(h.Imports, ", "))
		}
	}
	sb.WriteString("\n")
	for _, n := range b.Notes {
		fmt.Fprintf(&sb, "note: %s\n", n)
	}
	if b.Truncated {
		fmt.Fprintf(&sb, "trimmed to fit %s tokens\n", tokens.Format(b.Budget))
	}
	return sb.String()
}

func displayName(h Hit) string {
	if h.Package != "" {
		return h.Package + "." + h.Name
	}
	return h.Name
}
