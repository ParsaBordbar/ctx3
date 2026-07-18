package skillwriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sampleSkill() Skill {
	return Skill{
		Name:        "proj-deps",
		Description: "Dependency chain facts. Use when asked about imports or cycles.",
		Overview:    "Facts for proj.",
		References: []Reference{
			{Filename: "dependencies.md", When: "you need the full graph", Content: "GRAPH"},
		},
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		mutate  func(*Skill)
		wantErr bool
	}{
		"valid":            {func(s *Skill) {}, false},
		"bad name":         {func(s *Skill) { s.Name = "Proj_Deps" }, true},
		"empty name":       {func(s *Skill) { s.Name = "" }, true},
		"empty desc":       {func(s *Skill) { s.Description = "  " }, true},
		"ref with path":    {func(s *Skill) { s.References[0].Filename = "sub/x.md" }, true},
		"dup ref filename": {func(s *Skill) { s.References = append(s.References, s.References[0]) }, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := sampleSkill()
			c.mutate(&s)
			err := s.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("wantErr=%v got %v", c.wantErr, err)
			}
		})
	}
}

func TestRenderSkillMD(t *testing.T) {
	md := RenderSkillMD(sampleSkill())
	for _, want := range []string{
		"---\nname: proj-deps\n",
		"description: Dependency chain facts.",
		"## Reference files",
		"`reference/dependencies.md` — you need the full graph",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("SKILL.md missing %q\n---\n%s", want, md)
		}
	}
	// Frontmatter must be the very first bytes (Claude parses it strictly).
	if !strings.HasPrefix(md, "---\n") {
		t.Error("SKILL.md must start with frontmatter delimiter")
	}
}

func TestWrite_MaterializesTree(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".claude", "skills")

	got, files, err := Write(Config{SkillsDir: skillsDir}, sampleSkill())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files (SKILL.md + 1 ref), got %d: %v", len(files), files)
	}
	if _, err := os.Stat(filepath.Join(got, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not written: %v", err)
	}
	ref, err := os.ReadFile(filepath.Join(got, "reference", "dependencies.md"))
	if err != nil || string(ref) != "GRAPH" {
		t.Errorf("reference content wrong: %q err=%v", ref, err)
	}
}

func TestWrite_GuardAndForce(t *testing.T) {
	skillsDir := filepath.Join(t.TempDir(), "skills")
	if _, _, err := Write(Config{SkillsDir: skillsDir}, sampleSkill()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Write(Config{SkillsDir: skillsDir}, sampleSkill()); err == nil {
		t.Fatal("want guard error on existing dir")
	}
	if _, _, err := Write(Config{SkillsDir: skillsDir, Force: true}, sampleSkill()); err != nil {
		t.Fatalf("--force should overwrite: %v", err)
	}
}

func TestWrite_RejectsEmptySkillsDir(t *testing.T) {
	if _, _, err := Write(Config{SkillsDir: ""}, sampleSkill()); err == nil {
		t.Fatal("want error when SkillsDir is empty")
	}
}
