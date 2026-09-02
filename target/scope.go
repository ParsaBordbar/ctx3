package target

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Scope is where a skill lives on disk. When two scopes hold a skill with the
// same name, the one with the lower Priority wins — this mirrors Claude Code's
// resolution order, so `ctx3` can report which copy an agent will actually load.
//
//	enterprise  managed settings, highest priority
//	personal    the user's home directory
//	project     the repository being worked on
//	plugin      installed plugins, lowest priority
type Scope struct {
	Name     string // selector value, e.g. "personal"
	Priority int    // lower wins on a name collision
	Desc     string // one line for help output
	Writable bool   // false for plugin dirs — owned by the plugin, not by ctx3
}

// Scopes is the resolution order, highest priority first.
var Scopes = []Scope{
	{Name: "enterprise", Priority: 0, Desc: "machine-wide managed skills (needs admin rights)", Writable: true},
	{Name: "personal", Priority: 1, Desc: "your home directory — available in every repo", Writable: true},
	{Name: "project", Priority: 2, Desc: "checked into this repository, shared with the team", Writable: true},
	{Name: "plugin", Priority: 3, Desc: "installed plugins (read-only)", Writable: false},
}

// DefaultScope is used when no --scope is given: skills generated from a repo's
// own facts belong to that repo.
const DefaultScope = "project"

// GetScope returns the scope for name, or (zero, false) if unknown.
func GetScope(name string) (Scope, bool) {
	for _, s := range Scopes {
		if s.Name == name {
			return s, true
		}
	}
	return Scope{}, false
}

// ScopeNames returns the scope selectors in resolution order.
func ScopeNames() []string {
	names := make([]string, len(Scopes))
	for i, s := range Scopes {
		names[i] = s.Name
	}
	return names
}

// managedRoot is the OS location of a coding agent's machine-wide config.
// It matches where Claude Code reads managed-settings.json from.
func managedRoot() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/ClaudeCode"
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		return filepath.Join(programData, "ClaudeCode")
	default:
		return "/etc/claude-code"
	}
}

// SkillDirFor resolves the on-disk skill root for tgt in the named scope.
// projectRoot is only used by the project scope ("." means the cwd).
//
// A target's SkillDir is a relative convention (".claude/skills"), so the same
// layout is reused under the repo, under $HOME, and under the managed root.
func SkillDirFor(tgt Target, scope, projectRoot string) (string, error) {
	if !tgt.SupportsSkills() {
		return "", fmt.Errorf("target %q does not support skills", tgt.Name)
	}
	sc, ok := GetScope(scope)
	if !ok {
		return "", fmt.Errorf("unknown scope %q (want: %s)", scope, strings.Join(ScopeNames(), "|"))
	}

	switch sc.Name {
	case "project":
		if projectRoot == "" {
			projectRoot = "."
		}
		return filepath.Join(projectRoot, tgt.SkillDir), nil
	case "personal":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate home directory for the personal scope: %w", err)
		}
		return filepath.Join(home, tgt.SkillDir), nil
	case "enterprise":
		return filepath.Join(managedRoot(), "skills"), nil
	case "plugin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate home directory for the plugin scope: %w", err)
		}
		return filepath.Join(home, ".claude", "plugins"), nil
	}
	return "", fmt.Errorf("scope %q has no directory mapping", scope)
}

// AllSkillDirs resolves every scope's skill root for tgt, in resolution order.
// Scopes that cannot be resolved (e.g. no home directory) are skipped rather
// than failing the caller — listing is best-effort by design.
func AllSkillDirs(tgt Target, projectRoot string) []ScopeDir {
	out := make([]ScopeDir, 0, len(Scopes))
	for _, sc := range Scopes {
		dir, err := SkillDirFor(tgt, sc.Name, projectRoot)
		if err != nil {
			continue
		}
		out = append(out, ScopeDir{Scope: sc, Dir: dir})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Scope.Priority < out[j].Scope.Priority })
	return out
}

// ScopeDir pairs a scope with its resolved directory.
type ScopeDir struct {
	Scope Scope
	Dir   string
}
