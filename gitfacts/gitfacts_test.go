package gitfacts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a throwaway git checkout and returns its path.
func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
		{"config", "commit.gpgsign", "false"},
	} {
		run(t, dir, args...)
	}
	return dir
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	run(t, dir, "add", "-A")
	run(t, dir, "commit", "-q", "-m", msg)
}

func TestAnalyze_NotARepo(t *testing.T) {
	_, err := Analyze(Config{RootDir: t.TempDir()})
	if !errors.Is(err, ErrNotARepo) {
		t.Fatalf("err = %v, want ErrNotARepo", err)
	}
}

func TestAnalyze_CommitsAndChurn(t *testing.T) {
	dir := repo(t)

	// hot.go is touched by every commit; cold.go by one.
	write(t, dir, "hot.go", "package p\n")
	write(t, dir, "cold.go", "package p\n")
	commit(t, dir, "first: add both")

	write(t, dir, "hot.go", "package p\n// two\n")
	commit(t, dir, "second: touch hot")

	write(t, dir, "hot.go", "package p\n// three\n")
	commit(t, dir, "third: touch hot again")

	rep, err := Analyze(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	if rep.Branch != "main" {
		t.Errorf("branch = %q, want main", rep.Branch)
	}
	if rep.Head == "" {
		t.Error("HEAD sha missing")
	}

	// The log format asks git for NUL separators via %x00; passing the raw byte
	// makes exec reject the command and silently yields no commits.
	if len(rep.Commits) != 3 {
		t.Fatalf("got %d commits, want 3: %+v", len(rep.Commits), rep.Commits)
	}
	if rep.Commits[0].Subject != "third: touch hot again" {
		t.Errorf("newest commit = %q", rep.Commits[0].Subject)
	}
	if rep.Commits[0].Author != "Test User" {
		t.Errorf("author = %q", rep.Commits[0].Author)
	}
	if len(rep.Commits[0].Date) != len("2006-01-02") {
		t.Errorf("date %q should be date-only", rep.Commits[0].Date)
	}

	if len(rep.Churn) == 0 {
		t.Fatal("no churn collected")
	}
	if rep.Churn[0].Path != "hot.go" || rep.Churn[0].Commits != 3 {
		t.Errorf("hottest file = %+v, want hot.go with 3", rep.Churn[0])
	}
	if rep.Authors != 1 {
		t.Errorf("authors = %d, want 1", rep.Authors)
	}
}

func TestAnalyze_UncommittedWorkingSet(t *testing.T) {
	dir := repo(t)
	write(t, dir, "tracked.go", "package p\n")
	commit(t, dir, "init")

	write(t, dir, "tracked.go", "package p\n// edited\n")
	write(t, dir, "brand-new.go", "package p\n")
	if err := os.Remove(filepath.Join(dir, "tracked.go")); err == nil {
		// Re-create it: the point is a modified file, not a deleted one.
		write(t, dir, "tracked.go", "package p\n// edited\n")
	}

	rep, err := Analyze(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, c := range rep.Uncommitted {
		got[c.Path] = c.Status
	}
	if got["tracked.go"] != "modified" {
		t.Errorf("tracked.go status = %q, want modified (%v)", got["tracked.go"], got)
	}
	if got["brand-new.go"] != "untracked" {
		t.Errorf("brand-new.go status = %q, want untracked (%v)", got["brand-new.go"], got)
	}
}

func TestAnalyze_CleanTreeHasNoChanges(t *testing.T) {
	dir := repo(t)
	write(t, dir, "a.go", "package p\n")
	commit(t, dir, "init")

	rep, err := Analyze(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Uncommitted) != 0 {
		t.Errorf("clean tree reported changes: %+v", rep.Uncommitted)
	}
	if !strings.Contains(RenderText(rep), "Working tree clean") {
		t.Error("clean tree should say so")
	}
}

func TestAnalyze_EmptyRepoIsNotAnError(t *testing.T) {
	// A freshly-initialized repo has no HEAD. That is an empty history, not a
	// failure — the tool still has to answer.
	rep, err := Analyze(Config{RootDir: repo(t)})
	if err != nil {
		t.Fatalf("empty repo should analyze: %v", err)
	}
	if len(rep.Commits) != 0 {
		t.Errorf("commits = %+v, want none", rep.Commits)
	}
}

func TestAnalyze_TopFilesCapsChurn(t *testing.T) {
	dir := repo(t)
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		write(t, dir, name, "package p\n")
	}
	commit(t, dir, "init")

	rep, err := Analyze(Config{RootDir: dir, TopFiles: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Churn) != 2 {
		t.Errorf("churn length = %d, want 2", len(rep.Churn))
	}
}

func TestRenderMarkdown_EscapesPipesInSubjects(t *testing.T) {
	rep := &Report{
		Branch:  "main",
		Commits: []Commit{{SHA: "abc", Date: "2026-01-01", Subject: "fix: a|b parsing", Author: "X"}},
	}
	out := RenderMarkdown(rep)
	if !strings.Contains(out, `a\|b`) {
		t.Errorf("pipe not escaped, table would break:\n%s", out)
	}
}

func TestStatusWord(t *testing.T) {
	cases := map[string]string{
		"??": "untracked", "M": "modified", "A": "added", "D": "deleted",
		"R": "renamed", "UU": "conflicted", "R100": "renamed",
	}
	for code, want := range cases {
		if got := statusWord(code); got != want {
			t.Errorf("statusWord(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestSkill_TriggersAndStalenessWarning(t *testing.T) {
	dir := repo(t)
	write(t, dir, "hot.go", "package p\n")
	commit(t, dir, "init")
	write(t, dir, "wip.go", "package p\n")

	rep, err := Analyze(Config{RootDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	skill := rep.Skill("myproj")

	if skill.Name != "myproj-history" {
		t.Errorf("skill name = %q, want myproj-history", skill.Name)
	}
	for _, trigger := range []string{"being worked on", "changed recently", "hot"} {
		if !strings.Contains(skill.Description, trigger) {
			t.Errorf("description should trigger on %q: %q", trigger, skill.Description)
		}
	}
	// This domain is a snapshot of a moving tree, so it has to say so.
	if !strings.Contains(skill.Overview, "stale") {
		t.Errorf("overview should warn about staleness: %q", skill.Overview)
	}
	if !strings.Contains(skill.Overview, "uncommitted change") {
		t.Errorf("overview should report the working set: %q", skill.Overview)
	}
	if len(skill.References) != 1 {
		t.Fatalf("want one reference, got %d", len(skill.References))
	}
	if !strings.Contains(skill.References[0].Content, "hot.go") {
		t.Error("reference should carry the churn table")
	}
	// An empty name still produces a usable skill.
	if fallback := rep.Skill(""); fallback.Name != "project-history" {
		t.Errorf("fallback name = %q", fallback.Name)
	}
}
