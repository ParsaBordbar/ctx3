package analyzer

import (
	"strings"
	"testing"
)

func TestParseGoModRequires_Block(t *testing.T) {
	mod := `module example.com/x

go 1.24

require (
	github.com/bmatcuk/doublestar/v4 v4.9.1
	github.com/spf13/cobra v1.9.1
)

require (
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/toon-format/toon-go v0.0.0-20251202084852-7ca0e27c4e8c
)
`
	deps := parseGoModRequires(mod)
	want := []string{
		"github.com/bmatcuk/doublestar/v4 v4.9.1",
		"github.com/spf13/cobra v1.9.1",
		"github.com/spf13/pflag v1.0.6",
		"github.com/toon-format/toon-go v0.0.0-20251202084852-7ca0e27c4e8c",
	}
	if len(deps) != len(want) {
		t.Fatalf("got %d deps, want %d: %v", len(deps), len(want), deps)
	}
	for i := range want {
		if deps[i] != want[i] {
			t.Errorf("dep %d = %q, want %q", i, deps[i], want[i])
		}
	}
	// The literal "(" must never leak in as a dependency.
	for _, d := range deps {
		if strings.Contains(d, "(") {
			t.Errorf("dependency contains stray paren: %q", d)
		}
	}
}

func TestParseGoModRequires_SingleLine(t *testing.T) {
	mod := "module example.com/x\n\ngo 1.24\n\nrequire github.com/spf13/cobra v1.9.1\n"
	deps := parseGoModRequires(mod)
	if len(deps) != 1 || deps[0] != "github.com/spf13/cobra v1.9.1" {
		t.Fatalf("single-line require not parsed: %v", deps)
	}
}
