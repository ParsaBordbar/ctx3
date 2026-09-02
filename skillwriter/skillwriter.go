package skillwriter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Bundle sub-directories, per the open skill layout. Keeping SKILL.md small and
// pushing detail down here is the whole point: the agent loads SKILL.md on a
// description match and reads a bundled file only when the task reaches for it.
const (
	ReferencesDir = "references" // documentation read on demand
	ScriptsDir    = "scripts"    // executable code — run, never read
	AssetsDir     = "assets"     // templates, images, fixtures
)

// MaxSkillMDLines is the soft ceiling on SKILL.md. Past this the file is
// carrying content that belongs in references/. Lint reports it; Write does not
// block on it.
const MaxSkillMDLines = 500

// Reference is one progressive-disclosure document loaded on demand.
type Reference struct {
	Filename string // e.g. "dependencies.md" — lives under references/
	When     string // one line: when the agent should read this file
	Content  string
}

// Script is executable code shipped with the skill. Its contents never enter
// the context window — the agent runs it and only the output costs tokens, so
// the generated SKILL.md tells the agent to run it rather than read it.
type Script struct {
	Filename string // e.g. "validate_env.sh" — lives under scripts/
	When     string // one line: when to run it
	Run      string // the exact command, e.g. "bash scripts/validate_env.sh"
	Content  string
}

// Asset is a template, fixture or binary blob the skill depends on. Assets are
// listed but never described as readable prose — they are inputs to scripts or
// files to copy.
type Asset struct {
	Filename string // lives under assets/
	What     string // one line: what it is / what consumes it
	Content  string
}

// Skill is a complete skill to materialize.
type Skill struct {
	Name        string // kebab-case, e.g. "ctx3-deps"
	Description string // the trigger: what it covers + when to use it
	Overview    string // short body prose (kept small — detail goes in references)

	// Optional frontmatter. Model pins the skill to a model tier; AllowedTools
	// restricts what the agent may call while the skill is active. An empty
	// AllowedTools means "no restriction" — which is the right default for a
	// fact-only skill whose content is already on disk.
	Model        string
	AllowedTools []string

	References []Reference
	Scripts    []Script
	Assets     []Asset
}

// Config controls where and how the skill is written.
type Config struct {
	SkillsDir string // skill root, e.g. ".claude/skills" (from target registry + scope)
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
	if s.Model != "" && strings.ContainsAny(s.Model, " \t\n") {
		return fmt.Errorf("model %q must be a single identifier (e.g. opus, sonnet, haiku)", s.Model)
	}
	for _, t := range s.AllowedTools {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("allowed-tools contains an empty entry")
		}
		if strings.Contains(t, ",") {
			return fmt.Errorf("allowed-tools entry %q contains a comma — pass one tool per entry", t)
		}
	}

	seen := make(map[string]bool)
	check := func(kind, dir, name string) error {
		if name == "" {
			return fmt.Errorf("%s with empty filename", kind)
		}
		if strings.ContainsAny(name, "/\\") {
			return fmt.Errorf("%s filename %q must be a bare name (no path)", kind, name)
		}
		key := dir + "/" + name
		if seen[key] {
			return fmt.Errorf("duplicate %s filename %q", kind, name)
		}
		seen[key] = true
		return nil
	}
	for _, r := range s.References {
		if err := check("reference", ReferencesDir, r.Filename); err != nil {
			return err
		}
	}
	for _, sc := range s.Scripts {
		if err := check("script", ScriptsDir, sc.Filename); err != nil {
			return err
		}
	}
	for _, a := range s.Assets {
		if err := check("asset", AssetsDir, a.Filename); err != nil {
			return err
		}
	}
	return nil
}

// Lint returns non-fatal quality warnings: things that make a skill expensive
// or hard to trigger without making it invalid.
func (s Skill) Lint() []string {
	var warns []string
	if n := strings.Count(RenderSkillMD(s), "\n") + 1; n > MaxSkillMDLines {
		warns = append(warns, fmt.Sprintf(
			"SKILL.md is %d lines (soft limit %d) — move detail into %s/ so it loads only when needed",
			n, MaxSkillMDLines, ReferencesDir))
	}
	if len(s.Description) < 40 {
		warns = append(warns, "description is very short — it is the only text the agent matches a task against")
	}
	for _, sc := range s.Scripts {
		if strings.TrimSpace(sc.Run) == "" {
			warns = append(warns, fmt.Sprintf("script %s has no Run command — the agent may read it instead of running it", sc.Filename))
		}
	}
	return warns
}

func RenderSkillMD(s Skill) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "name: %s\n", s.Name)
	fmt.Fprintf(&sb, "description: %s\n", strings.TrimSpace(s.Description))
	if m := strings.TrimSpace(s.Model); m != "" {
		fmt.Fprintf(&sb, "model: %s\n", m)
	}
	if len(s.AllowedTools) > 0 {
		fmt.Fprintf(&sb, "allowed-tools: %s\n", strings.Join(s.AllowedTools, ", "))
	}
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
			fmt.Fprintf(&sb, "- `%s/%s` — %s\n", ReferencesDir, r.Filename, when)
		}
		sb.WriteString("\n")
	}

	if len(s.Scripts) > 0 {
		sb.WriteString("## Scripts\n\n")
		sb.WriteString("Run these — do not read them. Only their output enters the context window:\n\n")
		for _, sc := range s.Scripts {
			run := strings.TrimSpace(sc.Run)
			if run == "" {
				run = fmt.Sprintf("bash %s/%s", ScriptsDir, sc.Filename)
			}
			when := strings.TrimSpace(sc.When)
			if when == "" {
				when = "as needed"
			}
			fmt.Fprintf(&sb, "- `%s` — %s\n", run, when)
		}
		sb.WriteString("\n")
	}

	if len(s.Assets) > 0 {
		sb.WriteString("## Assets\n\n")
		for _, a := range s.Assets {
			what := strings.TrimSpace(a.What)
			if what == "" {
				what = "bundled data file"
			}
			fmt.Fprintf(&sb, "- `%s/%s` — %s\n", AssetsDir, a.Filename, what)
		}
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func Write(cfg Config, s Skill) (dir string, files []string, err error) {
	if err = s.Validate(); err != nil {
		return "", nil, err
	}
	if cfg.SkillsDir == "" {
		return "", nil, fmt.Errorf("SkillsDir is empty — the selected target does not support skills")
	}

	dir = filepath.Join(cfg.SkillsDir, s.Name)
	if _, statErr := os.Stat(dir); statErr == nil {
		if !cfg.Force {
			return "", nil, fmt.Errorf("%s already exists (use --force to overwrite)", dir)
		}
		// A refresh replaces the bundle wholesale. Files left behind by an older
		// generation (or by the reference/ → references/ rename) would otherwise
		// keep feeding the agent facts that no longer hold.
		for _, sub := range []string{ReferencesDir, ScriptsDir, AssetsDir, "reference"} {
			if err = os.RemoveAll(filepath.Join(dir, sub)); err != nil {
				return "", nil, err
			}
		}
	}

	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}

	skillPath := filepath.Join(dir, "SKILL.md")
	if err = os.WriteFile(skillPath, []byte(RenderSkillMD(s)), 0o644); err != nil {
		return "", nil, err
	}
	files = append(files, skillPath)

	write := func(sub, name, content string, mode os.FileMode) error {
		subDir := filepath.Join(dir, sub)
		if mkErr := os.MkdirAll(subDir, 0o755); mkErr != nil {
			return mkErr
		}
		p := filepath.Join(subDir, name)
		if wErr := os.WriteFile(p, []byte(content), mode); wErr != nil {
			return wErr
		}
		files = append(files, p)
		return nil
	}

	for _, r := range s.References {
		if err = write(ReferencesDir, r.Filename, r.Content, 0o644); err != nil {
			return "", nil, err
		}
	}
	for _, sc := range s.Scripts {
		// Executable: the agent runs these, it does not read them.
		if err = write(ScriptsDir, sc.Filename, sc.Content, 0o755); err != nil {
			return "", nil, err
		}
	}
	for _, a := range s.Assets {
		if err = write(AssetsDir, a.Filename, a.Content, 0o644); err != nil {
			return "", nil, err
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
