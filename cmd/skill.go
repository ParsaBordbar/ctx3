package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/target"
	"github.com/spf13/pflag"
)

// defaultSkillTarget is used when --as is empty: skills are a Claude Code convention.
const defaultSkillTarget = "claude"

// Shared skill flags. Every skill-capable command binds the same vars, so
// --scope/--model/--allowed-tools behave identically everywhere. Only one
// command runs per process, so a single set of vars is safe.
var (
	skillEmit         bool // --skill: emit instead of printing
	skillAs           string
	skillForce        bool
	skillScope        string
	skillModel        string
	skillAllowedTools []string
)

// bindSkillFlags wires the flags shared by every command that writes a skill.
func bindSkillFlags(f *pflag.FlagSet) {
	bindSkillInstallFlags(f)
	// No backquotes in these usage strings: pflag reads a backquoted word as the
	// argument placeholder, which turned "--model" into "--model model:".
	f.StringVar(&skillModel, "model", "", "Optional model: frontmatter for the skill (e.g. opus, sonnet, haiku)")
	f.StringSliceVar(&skillAllowedTools, "allowed-tools", nil, "Optional allowed-tools: frontmatter — an allowlist; omit for no restriction")
}

// bindSkillEmitFlags adds --skill to the shared set, for the analysis commands
// that can emit their facts as a skill instead of printing them.
func bindSkillEmitFlags(f *pflag.FlagSet) {
	f.BoolVar(&skillEmit, "skill", false, "Emit a coding-agent skill (SKILL.md + bundle) instead of printing; use -o - to preview")
	bindSkillFlags(f)
}

// bindSkillInstallFlags is the subset that also makes sense when copying a
// hand-written bundle, where model and allowed-tools belong to its author.
func bindSkillInstallFlags(f *pflag.FlagSet) {
	f.StringVar(&skillAs, "as", "", "Target tool convention (default: claude)")
	f.BoolVar(&skillForce, "force", false, "Overwrite an existing skill directory")
	f.StringVar(&skillScope, "scope", target.DefaultScope, "Install scope: "+strings.Join(target.ScopeNames(), "|"))
}

// resolveSkillTarget maps an --as selector to a skill-capable target. An empty
// selector falls back to defaultSkillTarget.
func resolveSkillTarget(as string) (target.Target, error) {
	sel := as
	if sel == "" {
		sel = defaultSkillTarget
	}
	tgt, ok := target.Get(sel)
	if !ok {
		return target.Target{}, fmt.Errorf("unknown --as value %q (want: %v)", as, target.Names())
	}
	if !tgt.SupportsSkills() {
		return target.Target{}, fmt.Errorf("target %q does not support skills (skills are a Claude Code convention)", sel)
	}
	return tgt, nil
}

// resolveSkillDir resolves target + scope to the directory skills are written
// to. projectRoot only matters for the project scope.
func resolveSkillDir(as, projectRoot string) (target.Target, string, error) {
	tgt, err := resolveSkillTarget(as)
	if err != nil {
		return target.Target{}, "", err
	}
	scope := skillScope
	if scope == "" {
		scope = target.DefaultScope
	}
	sc, ok := target.GetScope(scope)
	if !ok {
		return target.Target{}, "", fmt.Errorf("unknown --scope %q (want: %s)", scope, strings.Join(target.ScopeNames(), "|"))
	}
	if !sc.Writable {
		return target.Target{}, "", fmt.Errorf("scope %q is read-only — plugin skills are owned by the plugin that ships them", scope)
	}
	dir, err := target.SkillDirFor(tgt, scope, projectRoot)
	if err != nil {
		return target.Target{}, "", err
	}
	return tgt, dir, nil
}

// applySkillFlags stamps the shared optional frontmatter onto a generated skill.
func applySkillFlags(s *skillwriter.Skill) {
	if skillModel != "" {
		s.Model = skillModel
	}
	if len(skillAllowedTools) > 0 {
		s.AllowedTools = skillAllowedTools
	}
}

// refreshScript is the regeneration script shipped inside every generated
// skill. Generated facts rot as the code changes, and re-deriving them should
// not cost the agent a single token of context: it runs this, it never reads
// it, and only ctx3's own summary line comes back.
//
// The final `exec` matters — ctx3 overwrites this very file while it runs, and
// exec'ing means the shell has no reason to read past it.
func refreshScript(command, projectRoot string) skillwriter.Script {
	root := projectRoot
	if abs, err := filepath.Abs(projectRoot); err == nil {
		root = abs
	}
	scope := skillScope
	if scope == "" {
		scope = target.DefaultScope
	}

	args := []string{command, shellQuote(root), "--skill", "--force", "--scope", scope}
	if skillModel != "" {
		args = append(args, "--model", shellQuote(skillModel))
	}
	if len(skillAllowedTools) > 0 {
		args = append(args, "--allowed-tools", shellQuote(strings.Join(skillAllowedTools, ",")))
	}
	line := "ctx3 " + strings.Join(args, " ")

	content := "#!/bin/sh\n" +
		"# Regenerate this skill from the current checkout. Run it; there is nothing\n" +
		"# to read here. Requires ctx3 on PATH.\n" +
		"set -eu\n" +
		"exec " + line + "\n"

	return skillwriter.Script{
		Filename: "refresh.sh",
		When:     "these facts look stale — the code changed since this skill was generated",
		Run:      "sh " + skillwriter.ScriptsDir + "/refresh.sh",
		Content:  content,
	}
}

// shellQuote wraps a value for /bin/sh so paths with spaces survive.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// emitSkill resolves target + scope and materializes skill under the resulting
// skill dir. command is the ctx3 subcommand that produced the facts, embedded as
// a refresh script. When outputPath is "-" the rendered SKILL.md is printed to
// stdout instead, so callers can preview without touching the filesystem.
func emitSkill(command string, skill skillwriter.Skill, projectRoot, outputPath string) error {
	applySkillFlags(&skill)
	skill.Scripts = append(skill.Scripts, refreshScript(command, projectRoot))

	if outputPath == "-" {
		if err := skill.Validate(); err != nil {
			return err
		}
		fmt.Print(skillwriter.RenderSkillMD(skill))
		return nil
	}

	tgt, skillsDir, err := resolveSkillDir(skillAs, projectRoot)
	if err != nil {
		return err
	}

	skillDir, files, err := skillwriter.Write(skillwriter.Config{SkillsDir: skillsDir, Force: skillForce}, skill)
	if err != nil {
		return err
	}
	for _, w := range skill.Lint() {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", w)
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote skill %q (%d files) to %s [%s scope]\n", skill.Name, len(files), skillDir, skillScope)
	fmt.Fprintf(os.Stderr, "  %s will load it when a task matches its description. Re-run with --force to refresh.\n", tgt.Name)
	warnShadowed(tgt, skill.Name, projectRoot)
	return nil
}

// listScopeSkills lists one scope's installed skills. Plugin skills live under
// a nested layout and are namespaced `plugin:skill`, so they need their own walk.
func listScopeSkills(sd target.ScopeDir) ([]skillwriter.Entry, error) {
	if sd.Scope.Name == "plugin" {
		return skillwriter.ListPlugins(sd.Dir)
	}
	return skillwriter.List(sd.Dir)
}

// warnShadowed reports when a higher-priority scope already holds a skill with
// this name — the copy just written would never be the one that loads.
func warnShadowed(tgt target.Target, name, projectRoot string) {
	sc, ok := target.GetScope(skillScope)
	if !ok {
		return
	}
	for _, sd := range target.AllSkillDirs(tgt, projectRoot) {
		if sd.Scope.Priority >= sc.Priority {
			continue
		}
		entries, err := listScopeSkills(sd)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name == name {
				fmt.Fprintf(os.Stderr, "⚠ shadowed: %q also exists in the %s scope (%s), which wins. Rename this one to make it load.\n",
					name, sd.Scope.Name, e.Dir)
			}
		}
	}
}

// projectBaseName returns the go.mod module base, else the directory base name.
func projectBaseName(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "module "); ok {
				mod := strings.TrimSpace(rest)
				if i := strings.LastIndex(mod, "/"); i >= 0 {
					return mod[i+1:]
				}
				return mod
			}
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Base(abs)
	}
	return "project"
}
