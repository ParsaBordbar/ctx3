package version

import "testing"

func TestVersion_NeverEmpty(t *testing.T) {
	if Version() == "" {
		t.Fatal("Version() must never return an empty string")
	}
}

func TestVersion_LdflagOverride(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })
	version = "v9.9.9"
	if got := Version(); got != "v9.9.9" {
		t.Fatalf("ldflag override not honored: got %q", got)
	}
}
