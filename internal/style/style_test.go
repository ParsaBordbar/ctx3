package style

import (
	"os"
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
