package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_RejectsNonexistentDir(t *testing.T) {
	err := initCmd.RunE(initCmd, []string{filepath.Join(t.TempDir(), "nope")})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("want not-a-directory error, got %v", err)
	}
}

func TestInitCmd_RejectsBadPreset(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { initAs = "" })
	initAs = "bogus"
	err := initCmd.RunE(initCmd, []string{dir})
	if err == nil || !strings.Contains(err.Error(), "unknown --as") {
		t.Fatalf("want unknown-preset error, got %v", err)
	}
}

func TestInitCmd_GuardsExistingFile(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "AGENT.md")
	if err := os.WriteFile(out, []byte("keep me"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { initOutputPath, initForce = "", false })
	initOutputPath = out

	if err := initCmd.RunE(initCmd, []string{dir}); err == nil {
		t.Fatal("want error when output file exists without --force")
	}
	if data, _ := os.ReadFile(out); string(data) != "keep me" {
		t.Fatalf("existing file was clobbered: %q", data)
	}

	initForce = true
	if err := initCmd.RunE(initCmd, []string{dir}); err != nil {
		t.Fatalf("--force should overwrite, got %v", err)
	}
	if data, _ := os.ReadFile(out); string(data) == "keep me" {
		t.Fatal("--force did not overwrite the file")
	}
}
