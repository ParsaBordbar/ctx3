package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// authorSkill writes a minimal hand-authored skill and returns its directory.
func authorSkill(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\nname: " + name + "\ndescription: Custom skill. Use when the test asks for it.\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func resetAddSkillFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		skillScope, addSkillName, addSkillRoot, skillForce, addSkillDry, skillAs = "", "", "", false, false, ""
	})
}

func TestAddSkillCmd_InstallsIntoProjectScope(t *testing.T) {
	resetAddSkillFlags(t)
	src := authorSkill(t, "my-skill")
	repo := t.TempDir()
	skillScope, addSkillRoot = "project", repo

	if err := addSkillCmd.RunE(addSkillCmd, []string{src}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
		t.Fatalf("skill not installed under the project scope: %v", err)
	}

	// A second install must refuse rather than silently clobber.
	if err := addSkillCmd.RunE(addSkillCmd, []string{src}); err == nil {
		t.Error("want an error on an existing skill without --force")
	}
	skillForce = true
	if err := addSkillCmd.RunE(addSkillCmd, []string{src}); err != nil {
		t.Errorf("--force should overwrite: %v", err)
	}
}

func TestAddSkillCmd_PersonalScopeUsesHomeNotRepo(t *testing.T) {
	resetAddSkillFlags(t)
	src := authorSkill(t, "my-skill")
	home := t.TempDir()
	repo := t.TempDir()
	t.Setenv("HOME", home)
	skillScope, addSkillRoot = "personal", repo

	if err := addSkillCmd.RunE(addSkillCmd, []string{src}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md")); err != nil {
		t.Fatalf("personal scope should write under $HOME: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude")); err == nil {
		t.Error("personal scope must not touch the repository")
	}
}

func TestAddSkillCmd_RejectsPluginScope(t *testing.T) {
	resetAddSkillFlags(t)
	src := authorSkill(t, "my-skill")
	skillScope = "plugin"

	err := addSkillCmd.RunE(addSkillCmd, []string{src})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("want read-only error for the plugin scope, got %v", err)
	}
}

func TestAddSkillCmd_DryRunWritesNothing(t *testing.T) {
	resetAddSkillFlags(t)
	src := authorSkill(t, "my-skill")
	repo := t.TempDir()
	skillScope, addSkillRoot, addSkillDry = "project", repo, true

	if err := addSkillCmd.RunE(addSkillCmd, []string{src}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".claude")); err == nil {
		t.Error("--dry-run must not write")
	}
}

func TestAddSkillCmd_RejectsUndescribedSkill(t *testing.T) {
	resetAddSkillFlags(t)
	dir := filepath.Join(t.TempDir(), "no-desc")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: no-desc\ndescription:\n---\n"), 0o644)
	skillScope, addSkillRoot = "project", t.TempDir()

	err := addSkillCmd.RunE(addSkillCmd, []string{dir})
	if err == nil || !strings.Contains(err.Error(), "description") {
		t.Fatalf("an empty description must be rejected, got %v", err)
	}
}
