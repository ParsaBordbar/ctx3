package target

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScopePriorityOrder(t *testing.T) {
	// The order is the contract: enterprise beats personal beats project beats
	// plugin, matching how the agent resolves a name collision.
	want := []string{"enterprise", "personal", "project", "plugin"}
	got := ScopeNames()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("scope order = %v, want %v", got, want)
	}
	for i := 1; i < len(Scopes); i++ {
		if Scopes[i-1].Priority >= Scopes[i].Priority {
			t.Errorf("priorities not strictly increasing at %s", Scopes[i].Name)
		}
	}
	if p, _ := GetScope("plugin"); p.Writable {
		t.Error("plugin scope must not be writable — plugins own their skills")
	}
}

func TestSkillDirFor(t *testing.T) {
	claude, _ := Get("claude")

	got, err := SkillDirFor(claude, "project", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join("/repo", ".claude", "skills") {
		t.Errorf("project scope = %s", got)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	got, err = SkillDirFor(claude, "personal", "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(home, ".claude", "skills") {
		t.Errorf("personal scope = %s, want it under %s", got, home)
	}
	if got == filepath.Join("/repo", ".claude", "skills") {
		t.Error("personal scope must not resolve into the repo")
	}

	if _, err := SkillDirFor(claude, "nope", "."); err == nil {
		t.Error("want error on unknown scope")
	}
	agent, _ := Get("agent")
	if _, err := SkillDirFor(agent, "project", "."); err == nil {
		t.Error("a target without a skill dir must error")
	}
}

func TestAllSkillDirs(t *testing.T) {
	claude, _ := Get("claude")
	dirs := AllSkillDirs(claude, ".")
	if len(dirs) == 0 {
		t.Fatal("want at least the project scope")
	}
	for i := 1; i < len(dirs); i++ {
		if dirs[i-1].Scope.Priority > dirs[i].Scope.Priority {
			t.Fatalf("dirs not in resolution order: %v", dirs)
		}
	}
}
