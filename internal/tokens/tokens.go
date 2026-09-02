package tokens

import (
	"fmt"
	"strings"
)

const bytesPerToken = 4

func Estimate(s string) int {
	return (len(s) + bytesPerToken - 1) / bytesPerToken
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
	marker := fmt.Sprintf("\n… truncated to fit %s tokens\n", Format(budget))
	limit := budget*bytesPerToken - len(marker)
	if limit <= 0 {
		return marker, true
	}
	cut := strings.LastIndexByte(s[:limit], '\n')
	if cut <= 0 {
		cut = limit
	}
	return s[:cut] + marker, true
}
