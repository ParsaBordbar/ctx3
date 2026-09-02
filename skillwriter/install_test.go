package skillwriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkillTree(t *testing.T, frontmatter string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "my-skill")
	if err := os.MkdirAll(filepath.Join(dir, ReferencesDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ScriptsDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(frontmatter), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ReferencesDir, "guide.md"), []byte("GUIDE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ScriptsDir, "check.sh"), []byte("echo ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

const goodFrontmatter = `---
name: my-skill
description: Does a thing. Use when the user asks for that thing.
allowed-tools: Read, Grep
---

Body.
`

func TestParseMeta(t *testing.T) {
	m, err := ParseMeta(goodFrontmatter)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "my-skill" {
		t.Errorf("name = %q", m.Name)
	}
	if !strings.HasPrefix(m.Description, "Does a thing.") {
		t.Errorf("description = %q", m.Description)
	}
	if m.Extra["allowed-tools"] != "Read, Grep" {
		t.Errorf("extra frontmatter dropped: %v", m.Extra)
	}
}

func TestParseMeta_FoldedDescription(t *testing.T) {
	m, err := ParseMeta("---\nname: x-y\ndescription: first line\n  continued here\n---\n\nbody\n")
	if err != nil {
		t.Fatal(err)
	}
	if m.Description != "first line continued here" {
		t.Errorf("folded description = %q", m.Description)
	}
}

func TestParseMeta_BlockScalar(t *testing.T) {
	// Plugin skills often use `description: >` with the text on following lines.
	m, err := ParseMeta("---\nname: x-y\ndescription: >\n  Ultra-compressed mode.\n  Use when asked.\n---\n")
	if err != nil {
		t.Fatal(err)
	}
	if m.Description != "Ultra-compressed mode. Use when asked." {
		t.Errorf("block scalar description = %q", m.Description)
	}
}

func TestParseMeta_Errors(t *testing.T) {
	for name, src := range map[string]string{
		"no frontmatter": "# just markdown\n",
		"unterminated":   "---\nname: x\n",
	} {
		if _, err := ParseMeta(src); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestLoad_ValidatesTrigger(t *testing.T) {
	dir := writeSkillTree(t, goodFrontmatter)
	src, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Files) != 3 {
		t.Errorf("want SKILL.md + 2 bundled files, got %v", src.Files)
	}

	// The SKILL.md path is accepted too.
	if _, err := Load(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("loading SKILL.md directly: %v", err)
	}

	// An empty description makes the skill untriggerable — that is fatal.
	noDesc := writeSkillTree(t, "---\nname: my-skill\ndescription:\n---\n")
	if _, err := Load(noDesc); err == nil {
		t.Error("want error on empty description")
	}

	badName := writeSkillTree(t, "---\nname: My_Skill\ndescription: something useful here\n---\n")
	if _, err := Load(badName); err == nil {
		t.Error("want error on non-kebab name")
	}
}

func TestInstall_CopiesBundleAndRenames(t *testing.T) {
	src, err := Load(writeSkillTree(t, goodFrontmatter))
	if err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")

	dir, files, err := Install(Config{SkillsDir: skillsDir}, src, "backend-review")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "backend-review" {
		t.Errorf("installed dir = %s", dir)
	}
	if len(files) != 3 {
		t.Errorf("want 3 files copied, got %v", files)
	}
	if b, err := os.ReadFile(filepath.Join(dir, ReferencesDir, "guide.md")); err != nil || string(b) != "GUIDE" {
		t.Errorf("reference not copied: %q %v", b, err)
	}
	// The frontmatter must follow the rename or the agent indexes the old name.
	md, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "name: backend-review") {
		t.Errorf("frontmatter name not rewritten:\n%s", md)
	}

	// Second install without --force must refuse.
	if _, _, err := Install(Config{SkillsDir: skillsDir}, src, "backend-review"); err == nil {
		t.Error("want guard error on existing skill")
	}
	if _, _, err := Install(Config{SkillsDir: skillsDir, Force: true}, src, "backend-review"); err != nil {
		t.Errorf("--force should overwrite: %v", err)
	}
}

func TestInstall_ScriptsStayExecutable(t *testing.T) {
	src, err := Load(writeSkillTree(t, goodFrontmatter))
	if err != nil {
		t.Fatal(err)
	}
	dir, _, err := Install(Config{SkillsDir: filepath.Join(t.TempDir(), "skills")}, src, "")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, ScriptsDir, "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script lost its exec bit: %v — the agent must be able to run it", info.Mode())
	}
}

func TestList_ReportsInstalledAndBroken(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good-skill")
	os.MkdirAll(good, 0o755)
	os.WriteFile(filepath.Join(good, "SKILL.md"), []byte("---\nname: good-skill\ndescription: a trigger\n---\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "empty-dir"), 0o755)

	entries, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %v", entries)
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if byName["good-skill"].Description != "a trigger" {
		t.Errorf("description not read: %+v", byName["good-skill"])
	}
	if byName["empty-dir"].Err == nil {
		t.Error("a dir without SKILL.md should be flagged")
	}

	// A missing scope dir is normal, not an error.
	if got, err := List(filepath.Join(root, "nope")); err != nil || got != nil {
		t.Errorf("missing dir should be empty: %v %v", got, err)
	}
}

func TestRenderSkillMD_OptionalFrontmatterAndBundles(t *testing.T) {
	s := sampleSkill()
	s.Model = "opus"
	s.AllowedTools = []string{"Read", "Bash(ctx3:*)"}
	s.Scripts = []Script{{Filename: "check.sh", When: "before answering", Run: "bash scripts/check.sh", Content: "echo ok"}}
	s.Assets = []Asset{{Filename: "template.json", What: "config template to copy"}}

	md := RenderSkillMD(s)
	for _, want := range []string{
		"model: opus\n",
		"allowed-tools: Read, Bash(ctx3:*)\n",
		"Run these — do not read them",
		"`bash scripts/check.sh` — before answering",
		"`assets/template.json` — config template to copy",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q\n---\n%s", want, md)
		}
	}

	dir, files, err := Write(Config{SkillsDir: filepath.Join(t.TempDir(), "skills")}, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Errorf("want SKILL.md + ref + script + asset, got %v", files)
	}
	info, err := os.Stat(filepath.Join(dir, ScriptsDir, "check.sh"))
	if err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Errorf("generated script must be executable: %v %v", info, err)
	}
}

func TestLint(t *testing.T) {
	s := sampleSkill()
	s.Overview = strings.Repeat("line\n", MaxSkillMDLines+10)
	warns := strings.Join(s.Lint(), " ")
	if !strings.Contains(warns, "soft limit") {
		t.Errorf("oversized SKILL.md should warn, got %q", warns)
	}

	s2 := sampleSkill()
	s2.Scripts = []Script{{Filename: "x.sh", Content: "true"}}
	if !strings.Contains(strings.Join(s2.Lint(), " "), "no Run command") {
		t.Error("script without a Run command should warn")
	}
}

func TestValidate_OptionalFields(t *testing.T) {
	s := sampleSkill()
	s.AllowedTools = []string{"Read, Grep"}
	if err := s.Validate(); err == nil {
		t.Error("a comma inside one allowed-tools entry should be rejected")
	}
	s2 := sampleSkill()
	s2.Model = "claude opus"
	if err := s2.Validate(); err == nil {
		t.Error("a model with whitespace should be rejected")
	}
}
