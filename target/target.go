// Package target is the single source of truth for which coding-agent tool
// ctx3 generates output for. Selecting a target determines the context-file
// name, whether skills are supported, and where they live. Both `init` and the
// skill-emitting commands (deps, db, ...) consume this registry so naming stays
// consistent across the tool.
package target

import "sort"

// Target describes one coding-agent tool's context conventions.
type Target struct {
	Name        string // selector value, e.g. "claude"
	ContextFile string // top-level context file, e.g. "CLAUDE.md" ("" = none)
	SkillDir    string // skill root, e.g. ".claude/skills" ("" = skills unsupported)
	RuleDir     string // rules dir, e.g. ".cursor/rules" ("" = none)
}

// SupportsSkills reports whether this target consumes progressive-disclosure skills.
func (t Target) SupportsSkills() bool { return t.SkillDir != "" }

// Registry maps selector value -> Target.
var Registry = map[string]Target{
	"agent":   {Name: "agent", ContextFile: "AGENTS.md"},
	"claude":  {Name: "claude", ContextFile: "CLAUDE.md", SkillDir: ".claude/skills"},
	"cursor":  {Name: "cursor", RuleDir: ".cursor/rules"},
	"copilot": {Name: "copilot", ContextFile: ".github/copilot-instructions.md"},
	"gemini":  {Name: "gemini", ContextFile: "GEMINI.md"},
}

// Default is used when no target is specified.
const Default = "agent"

// Get returns the target for name, or (zero, false) if unknown.
func Get(name string) (Target, bool) {
	t, ok := Registry[name]
	return t, ok
}

// Names returns the sorted list of known target selectors.
func Names() []string {
	names := make([]string, 0, len(Registry))
	for k := range Registry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
