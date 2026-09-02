package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/db"
	"github.com/parsabordbar/ctx3/deps"
	"github.com/parsabordbar/ctx3/flow"
	"github.com/parsabordbar/ctx3/gitfacts"
	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/symbols"
	"github.com/parsabordbar/ctx3/target"

	"github.com/spf13/cobra"
)

var (
	skillsOnly string
	skillsList bool
)

// skillDomains are the fact-emitters the umbrella command can materialize, in
// emit order. Each builds a skillwriter.Skill from a fresh analysis of the
// directory. None of them fail on a broken tree: the parse-only domains never
// needed a build, and "flow" degrades to parse-only analysis when the module
// does not type-check.
//
// funcs and impact are deliberately absent: `functions` is a more detailed view
// of what `symbols` already emits, and `impact` answers a per-symbol query, so
// neither has a fixed set of facts worth freezing into a file.
var skillDomains = []string{"overview", "symbols", "deps", "flow", "db", "git"}

// domainCommand maps a domain to the ctx3 subcommand that regenerates it, so
// each emitted skill can ship a refresh script the agent runs instead of
// remembering the flags.
var domainCommand = map[string]string{
	"overview": "context",
	"symbols":  "map",
	"deps":     "deps",
	"flow":     "flow",
	"db":       "db",
	"git":      "git",
}

var skillsCmd = &cobra.Command{
	Use:   "skills [directory]",
	Short: "Emit the full progressive-disclosure skill bundle for a coding agent",
	Long: `Generate every ctx3 fact-domain as a Claude Code skill in one shot:

  overview  project shape, deps, languages, entry points   (ctx3 context)
  symbols   symbol index with file:line                    (ctx3 map)
  deps      internal import graph + cycles                 (ctx3 deps)
  flow      call graph + entry points                      (ctx3 flow)
  db        detected datastores + relational schema         (ctx3 db)
  git       branch, work in progress, commits, hot files    (ctx3 git)

Each domain becomes a self-registering skill under the target's skill dir, with
a sharp description so the agent loads it only when a task matches. This is the
per-command --skill flags rolled into one bundle.

Every domain works on a partial checkout. The flow domain prefers a type-checked
graph and degrades to parse-only analysis when the module does not compile.

Skills install into a scope (--scope, default project). When the same name
exists in two scopes the higher-priority one wins: enterprise > personal >
project > plugin. Use --list to see every installed skill and what shadows what.

Examples:
  ctx3 skills .                        # all six skills, project scope
  ctx3 skills . --scope personal       # available in every repo
  ctx3 skills . --only symbols,deps    # a subset
  ctx3 skills . --force                # refresh existing skills
  ctx3 skills --list                   # what is installed, in priority order`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dirArg(args)

		if skillsList {
			return listSkills(skillAs, dir)
		}

		tgt, skillsDir, err := resolveSkillDir(skillAs, dir)
		if err != nil {
			return err
		}

		want, err := parseSkillDomains(skillsOnly)
		if err != nil {
			return err
		}

		name := projectBaseName(dir)
		cfg := skillwriter.Config{SkillsDir: skillsDir, Force: skillForce}

		var built, failed int
		for _, dom := range skillDomains {
			if !want[dom] {
				continue
			}
			skill, err := buildDomainSkill(dom, dir, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ %s: skipped (%v)\n", dom, err)
				failed++
				continue
			}
			applySkillFlags(skill)
			skill.Scripts = append(skill.Scripts, refreshScript(domainCommand[dom], dir))
			skillDir, files, err := skillwriter.Write(cfg, *skill)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠ %s: %v\n", dom, err)
				failed++
				continue
			}
			for _, w := range skill.Lint() {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: %s\n", dom, w)
			}
			fmt.Fprintf(os.Stderr, "✓ %s → skill %q (%d files) at %s\n", dom, skill.Name, len(files), skillDir)
			warnShadowed(tgt, skill.Name, dir)
			built++
		}

		fmt.Fprintf(os.Stderr, "\n%d skill(s) written to %s [%s scope]", built, skillsDir, skillScope)
		if failed > 0 {
			fmt.Fprintf(os.Stderr, ", %d skipped", failed)
		}
		fmt.Fprintf(os.Stderr, ". %s loads each when a task matches its description.\n", tgt.Name)
		return nil
	},
}

// listSkills prints every installed skill per scope in resolution order and
// marks the copies that lose a name collision.
func listSkills(as, projectRoot string) error {
	tgt, err := resolveSkillTarget(as)
	if err != nil {
		return err
	}

	winner := map[string]string{} // skill name -> scope that wins
	dirs := target.AllSkillDirs(tgt, projectRoot)

	for _, sd := range dirs {
		entries, err := listScopeSkills(sd)
		if err != nil {
			fmt.Printf("%s (%s): %v\n\n", sd.Scope.Name, sd.Dir, err)
			continue
		}
		fmt.Printf("%s — %s\n%s\n", strings.ToUpper(sd.Scope.Name), sd.Scope.Desc, sd.Dir)
		if len(entries) == 0 {
			fmt.Printf("  (none)\n\n")
			continue
		}
		for _, e := range entries {
			switch {
			case e.Err != nil:
				fmt.Printf("  ✘ %-28s %v\n", e.Name, e.Err)
			case winner[e.Name] != "":
				fmt.Printf("  ⊘ %-28s shadowed by the %s scope\n", e.Name, winner[e.Name])
			default:
				winner[e.Name] = sd.Scope.Name
				fmt.Printf("  ✓ %-28s %s\n", e.Name, truncate(e.Description, 70))
			}
		}
		fmt.Println()
	}
	return nil
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// buildDomainSkill runs the analysis behind one domain and returns its skill.
func buildDomainSkill(dom, dir, name string) (*skillwriter.Skill, error) {
	switch dom {
	case "overview":
		ctx := analyzer.AnalyzeProject(dir)
		s := ctx.Skill(name)
		return &s, nil
	case "symbols":
		idx, err := symbols.Scan(symbols.Config{Path: dir})
		if err != nil {
			return nil, err
		}
		s := idx.Skill(name)
		return &s, nil
	case "deps":
		g, err := deps.Analyze(deps.Config{RootDir: dir})
		if err != nil {
			return nil, err
		}
		s := g.Skill()
		return &s, nil
	case "flow":
		g, err := flow.AnalyzeFlow(flow.Config{RootDir: dir})
		if err != nil {
			return nil, err
		}
		s := g.Skill(name)
		return &s, nil
	case "db":
		rep, err := db.Analyze(db.Config{RootDir: dir})
		if err != nil {
			return nil, err
		}
		s := rep.Skill(name)
		return &s, nil
	case "git":
		rep, err := gitfacts.Analyze(gitfacts.Config{RootDir: dir})
		if err != nil {
			// A tarball or a non-git checkout has no history to emit. That is a
			// missing domain, not a failed bundle.
			return nil, err
		}
		s := rep.Skill(name)
		return &s, nil
	default:
		return nil, fmt.Errorf("unknown domain %q", dom)
	}
}

// parseSkillDomains turns "symbols,deps" into a set, defaulting to all domains.
func parseSkillDomains(list string) (map[string]bool, error) {
	want := make(map[string]bool)
	if strings.TrimSpace(list) == "" {
		for _, d := range skillDomains {
			want[d] = true
		}
		return want, nil
	}
	valid := make(map[string]bool)
	for _, d := range skillDomains {
		valid[d] = true
	}
	for raw := range strings.SplitSeq(list, ",") {
		d := strings.ToLower(strings.TrimSpace(raw))
		if d == "" {
			continue
		}
		if !valid[d] {
			return nil, fmt.Errorf("unknown domain %q (valid: %s)", d, strings.Join(skillDomains, "|"))
		}
		want[d] = true
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("--only listed no valid domains")
	}
	return want, nil
}

func init() {
	f := skillsCmd.Flags()
	f.StringVar(&skillsOnly, "only", "", "Comma-separated subset ("+strings.Join(skillDomains, "|")+")")
	f.BoolVarP(&skillsList, "list", "l", false, "List installed skills across all scopes, marking what shadows what")
	bindSkillFlags(f)
	rootCmd.AddCommand(skillsCmd)
}
