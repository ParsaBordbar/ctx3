package mascot

import (
	"strings"
	"testing"
)

func TestRender_Mono(t *testing.T) {
	out := Render(false)
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("mono render must not contain ANSI escapes")
	}
	if !strings.ContainsAny(out, "█▀▄") {
		t.Fatalf("mono render should draw block glyphs; got:\n%s", out)
	}
	// One text line per two pixel rows (rounded up).
	if want, got := (len(sprite)+1)/2, strings.Count(out, "\n"); got != want {
		t.Fatalf("expected %d lines, got %d", want, got)
	}
}

func TestRender_RowsShareWidth(t *testing.T) {
	lines := strings.Split(strings.TrimRight(Render(false), "\n"), "\n")
	want := len([]rune(lines[0]))
	for i, l := range lines {
		if got := len([]rune(l)); got != want {
			t.Fatalf("line %d is %d cells wide, want %d (short sprite row clipping?):\n%s",
				i, got, want, Render(false))
		}
	}
}

func TestRender_Color(t *testing.T) {
	out := Render(true)
	if !strings.Contains(out, "\x1b[38;2;67;181;230m") {
		t.Fatalf("color render should use the logo body blue")
	}
	if !strings.HasSuffix(strings.TrimRight(out, "\n"), reset) {
		t.Fatalf("each colored line should reset before newline")
	}
}

func TestBeside_AlignsRight(t *testing.T) {
	out := Beside("one\ntwo\nthree", 3, false)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Composed height matches the skull (one line per two pixel rows, rounded up);
	// the 3 text lines are centered within.
	wantLines := (len(sprite) + 1) / 2
	if len(lines) != wantLines {
		t.Fatalf("expected %d composed lines, got %d:\n%s", wantLines, len(lines), out)
	}
	for _, want := range []string{"   one", "   two", "   three"} {
		found := false
		for _, l := range lines {
			if strings.HasSuffix(l, want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no line ends with %q:\n%s", want, out)
		}
	}
}

func TestColorable_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if Colorable(nil) {
		t.Fatalf("NO_COLOR must disable color regardless of the writer")
	}
}
