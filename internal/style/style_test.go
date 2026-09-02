package style

import (
	"os"
	"strings"
	"testing"
)

func TestPlainIsIdentity(t *testing.T) {
	if got := Plain.Bold("x"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := ANSI.Bold(""); got != "" {
		t.Fatalf("empty must stay empty, got %q", got)
	}
}

func TestStripAndWidth(t *testing.T) {
	s := ANSI.Title("abc") + " " + ANSI.Dim("é")
	if Strip(s) != "abc é" {
		t.Fatalf("strip: %q", Strip(s))
	}
	if Width(s) != 5 {
		t.Fatalf("width: %d", Width(s))
	}
	if got := Ljust(ANSI.Ok("ab"), 4); Strip(got) != "ab  " {
		t.Fatalf("ljust: %q", got)
	}
}

func TestResolve(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if p, _ := Resolve("auto", os.Stdout); p.Enabled() {
		t.Fatal("NO_COLOR must disable auto")
	}
	if p, _ := Resolve("always", os.Stdout); !p.Enabled() {
		t.Fatal("always must enable")
	}
	if p, _ := Resolve("never", os.Stdout); p.Enabled() {
		t.Fatal("never must disable")
	}
	if _, err := Resolve("sometimes", os.Stdout); err == nil {
		t.Fatal("bad mode must error")
	}
}

func TestGlyphHonorsEmojiToggle(t *testing.T) {
	if got := Plain.Glyph("🚀", "[entry]"); got != "🚀" {
		t.Fatalf("default palette should keep emoji, got %q", got)
	}
	if got := Plain.WithEmoji(false).Glyph("🚀", "[entry]"); got != "[entry]" {
		t.Fatalf("WithEmoji(false) should return ascii, got %q", got)
	}
	if got := ANSI.WithEmoji(false).Ok("x"); !strings.Contains(got, "\x1b[") {
		t.Fatalf("WithEmoji must not disable color: %q", got)
	}
	if ANSI.WithEmoji(false).WithEmoji(true).Emoji() != true {
		t.Fatal("WithEmoji(true) should restore emoji")
	}
}
