package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitCommand_SkillPreview(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	td := t.TempDir()
	mustWrite(t, filepath.Join(td, "a.go"), []byte("package a\n"))
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "add", "."},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "first"},
	} {
		c := exec.Command("git", args...)
		c.Dir = td
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	rootCmd.SetArgs([]string{"git", td, "--skill", "-o", "-"})
	execErr := rootCmd.Execute()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	skillEmit = false

	if execErr != nil {
		t.Fatalf("execute: %v", execErr)
	}
	out := buf.String()
	for _, want := range []string{"---\nname: ", "description: Repository history", "ctx3 git"} {
		if !strings.Contains(out, want) {
			t.Errorf("SKILL.md preview missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(td, ".claude")); err == nil {
		t.Error("-o - must not write a skill directory")
	}
}
