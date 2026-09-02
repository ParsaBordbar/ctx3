package tokens

import (
	"strings"
	"testing"
)

func TestEstimateRoundsUp(t *testing.T) {
	if got := Estimate("abcde"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := Estimate(""); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestFormatThousands(t *testing.T) {
	for in, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567"} {
		if got := Format(in); got != want {
			t.Errorf("%d: got %q want %q", in, got, want)
		}
	}
}

func TestFitCutsOnLineBoundary(t *testing.T) {
	s := strings.Repeat("0123456789\n", 100)
	out, trunc := Fit(s, 50)
	if !trunc {
		t.Fatal("expected truncation")
	}
	if Estimate(out) > 50 {
		t.Fatalf("still over budget: %d", Estimate(out))
	}
	if !strings.Contains(out, "truncated to fit 50 tokens") {
		t.Fatalf("missing marker:\n%s", out)
	}
	body := out[:strings.Index(out, "\n… truncated")]
	if !strings.HasSuffix(body, "0123456789") {
		t.Fatalf("cut mid-line: %q", body[len(body)-12:])
	}
	if _, trunc := Fit(s, 0); trunc {
		t.Fatal("zero budget must mean unlimited")
	}
	if _, trunc := Fit("short", 100); trunc {
		t.Fatal("under budget must not truncate")
	}
}
