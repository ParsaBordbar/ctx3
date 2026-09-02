package tokens

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/internal/style"
)

const bytesPerToken = 4

func Estimate(s string) int {
	return (len(style.Strip(s)) + bytesPerToken - 1) / bytesPerToken
}

func Format(n int) string {
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func Line(s string) string {
	return "≈ " + Format(Estimate(s)) + " tokens"
}

func Fit(s string, budget int) (string, bool) {
	if budget <= 0 || Estimate(s) <= budget {
		return s, false
	}
	marker := fmt.Sprintf("… truncated to fit %s tokens\n", Format(budget))
	limit := budget*bytesPerToken - len(marker) - 1
	if limit <= 0 {
		return "\n" + marker, true
	}
	var out strings.Builder
	used := 0
	for _, line := range strings.SplitAfter(s, "\n") {
		n := len(style.Strip(line))
		if used+n > limit {
			break
		}
		out.WriteString(line)
		used += n
	}
	body := out.String()
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "\n" + marker, true
}
