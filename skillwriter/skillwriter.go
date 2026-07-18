package skillwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Reference is one progressive-disclosure file loaded on demand.
type Reference struct {
	Filename string // e.g. "dependencies.md" — lives under reference/
	When     string // one line: when Claude should read this file
	Content  string
}

// Skill is a complete skill to materialize.
type Skill struct {
	Name        string // kebab-case, e.g. "ctx3-deps"
	Description string // the trigger: what it covers + when to use it
	Overview    string // short body prose (kept small — detail goes in references)
	References  []Reference
}

// Config controls where and how the skill is written.
type Config struct {
	SkillsDir string // skill root, e.g. ".claude/skills" (from target registry)
	Force     bool   // overwrite an existing skill directory
}

// Validate checks the invariants that keep a skill useful.
func (s Skill) Validate() error {
	if !isKebab(s.Name) {
		return fmt.Errorf("skill name %q must be kebab-case (a-z, 0-9, -)", s.Name)
	}
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("skill %q needs a description — it is the trigger Claude matches on", s.Name)
	}
	seen := make(map[string]bool)
	for _, r := range s.References {
		if r.Filename == "" {
			return fmt.Errorf("reference with empty filename")
		}
		if strings.ContainsAny(r.Filename, "/\\") {
			return fmt.Errorf("reference filename %q must be a bare name (no path)", r.Filename)
		}
		if seen[r.Filename] {
			return fmt.Errorf("duplicate reference filename %q", r.Filename)
		}
		seen[r.Filename] = true
	}
	return nil
}


func RenderSkillMD(s Skill) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", s.Name)
	fmt.Fprintf(&sb, "description: %s\n", strings.TrimSpace(s.Description))
	sb.WriteString("---\n\n")

	if ov := strings.TrimSpace(s.Overview); ov != "" {
		sb.WriteString(ov)
		sb.WriteString("\n\n")
	}

	if len(s.References) > 0 {
		sb.WriteString("## Reference files\n\n")
		sb.WriteString("Load these on demand — do not read them until the task needs them:\n\n")
		for _, r := range s.References {
			when := strings.TrimSpace(r.When)
			if when == "" {
				when = "reference data"
			}
			fmt.Fprintf(&sb, "- `reference/%s` — %s\n", r.Filename, when)
		}
	}
	return sb.String()
}

func Write(cfg Config, s Skill) (dir string, files []string, err error) {
	if err = s.Validate(); err != nil {
		return "", nil, err
	}
	if cfg.SkillsDir == "" {
		return "", nil, fmt.Errorf("SkillsDir is empty — the selected target does not support skills")
	}

	dir = filepath.Join(cfg.SkillsDir, s.Name)
	if _, statErr := os.Stat(dir); statErr == nil && !cfg.Force {
		return "", nil, fmt.Errorf("%s already exists (use --force to overwrite)", dir)
	}

	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}

	skillPath := filepath.Join(dir, "SKILL.md")
	if err = os.WriteFile(skillPath, []byte(RenderSkillMD(s)), 0o644); err != nil {
		return "", nil, err
	}
	files = append(files, skillPath)

	if len(s.References) > 0 {
		refDir := filepath.Join(dir, "reference")
		if err = os.MkdirAll(refDir, 0o755); err != nil {
			return "", nil, err
		}
		for _, r := range s.References {
			p := filepath.Join(refDir, r.Filename)
			if err = os.WriteFile(p, []byte(r.Content), 0o644); err != nil {
				return "", nil, err
			}
			files = append(files, p)
		}
	}
	return dir, files, nil
}

func isKebab(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-':
			// no leading/trailing/double hyphen
			if i == 0 || i == len(s)-1 || s[i-1] == '-' {
				return false
			}
		default:
			return false
		}
	}
	return true
}
