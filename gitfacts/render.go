package gitfacts

import (
	"fmt"
	"github.com/parsabordbar/ctx3/internal/style"
	"strings"
)

// RenderText is the compact default: state first, then the working set, then
// what churns. Ordered by what a reader needs to know soonest.
func RenderText(r *Report) string { return RenderStyled(r, style.Plain) }

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

func RenderStyled(r *Report, st style.Palette) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "%s %s", st.Title("Repository —"), st.Bold(r.Branch))
	if r.Head != "" {
		fmt.Fprintf(&sb, " @ %s", st.Warn(r.Head))
	}
	if r.HeadDate != "" {
		fmt.Fprintf(&sb, " %s", st.Dim("("+r.HeadDate+")"))
	}
	sb.WriteByte('\n')
	if r.Remote != "" {
		fmt.Fprintf(&sb, "  %s %s\n", st.Dim("remote:"), r.Remote)
	}

	if len(r.Uncommitted) == 0 {
		sb.WriteString("\n" + st.Ok("Working tree clean.") + "\n")
	} else {
		fmt.Fprintf(&sb, "\n%s\n", st.Bold(fmt.Sprintf("Uncommitted (%d) — work in progress:", len(r.Uncommitted))))
		for _, c := range r.Uncommitted {
			fmt.Fprintf(&sb, "  %s %s\n", style.Ljust(statusColor(st, c.Status), 10), c.Path)
		}
	}

	if len(r.Commits) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", st.Bold(fmt.Sprintf("Recent commits (%d):", len(r.Commits))))
		for _, c := range r.Commits {
			fmt.Fprintf(&sb, "  %s  %s  %s %s\n", st.Warn(c.SHA), st.Dim(c.Date), c.Subject, st.Dim("("+c.Author+")"))
		}
	}

	if len(r.Churn) > 0 {
		fmt.Fprintf(&sb, "\n%s\n", st.Bold(fmt.Sprintf("Hot files — most changed over the last %d commits (%d author(s)):",
			r.ChurnWindow, r.Authors)))
		width := 0
		for _, f := range r.Churn {
			if n := len(f.Path); n > width {
				width = n
			}
		}
		if width > 60 {
			width = 60
		}
		for _, f := range r.Churn {
			fmt.Fprintf(&sb, "  %s  %s %s   %s\n", style.Ljust(st.Path(truncPath(f.Path, width)), width),
				st.Warn(fmt.Sprintf("%3d", f.Commits)), st.Dim("commits"), st.Dim("last "+f.LastChanged))
		}
	}

	return sb.String()
}

// RenderMarkdown is the same facts as tables, for embedding in a document.
func RenderMarkdown(r *Report) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Repository state\n\n- Branch: `%s`\n", r.Branch)
	if r.Head != "" {
		fmt.Fprintf(&sb, "- HEAD: `%s` (%s)\n", r.Head, r.HeadDate)
	}
	if r.Remote != "" {
		fmt.Fprintf(&sb, "- Remote: %s\n", r.Remote)
	}

	sb.WriteString("\n## Uncommitted\n\n")
	if len(r.Uncommitted) == 0 {
		sb.WriteString("Working tree clean.\n")
	} else {
		sb.WriteString("| Status | Path |\n|---|---|\n")
		for _, c := range r.Uncommitted {
			fmt.Fprintf(&sb, "| %s | `%s` |\n", c.Status, c.Path)
		}
	}

	if len(r.Commits) > 0 {
		sb.WriteString("\n## Recent commits\n\n| SHA | Date | Subject | Author |\n|---|---|---|---|\n")
		for _, c := range r.Commits {
			fmt.Fprintf(&sb, "| `%s` | %s | %s | %s |\n", c.SHA, c.Date, escapePipes(c.Subject), c.Author)
		}
	}

	if len(r.Churn) > 0 {
		fmt.Fprintf(&sb, "\n## Hot files (last %d commits)\n\n| Path | Commits | Last changed |\n|---|---|---|\n", r.ChurnWindow)
		for _, f := range r.Churn {
			fmt.Fprintf(&sb, "| `%s` | %d | %s |\n", f.Path, f.Commits, f.LastChanged)
		}
	}

	return sb.String()
}

// escapePipes keeps a commit subject from breaking the table it sits in.
func escapePipes(s string) string { return strings.ReplaceAll(s, "|", `\|`) }

// truncPath shortens from the left, keeping the file name visible.
func truncPath(p string, max int) string {
	if len(p) <= max {
		return p
	}
	if max <= 1 {
		return p[:max]
	}
	return "…" + p[len(p)-(max-1):]
}
