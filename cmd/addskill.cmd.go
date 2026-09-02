package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/target"

	"github.com/spf13/cobra"
)

var (
	addSkillName string
	addSkillDry  bool
	addSkillRoot string
)

var addSkillCmd = &cobra.Command{
	Use:     "add-skill <path>",
	Aliases: []string{"install-skill"},
	Short:   "Install a hand-written skill into a scope (personal, project, enterprise)",
	Long: `Take a skill you wrote by hand and install it where a coding agent will find it.

<path> is either the skill directory or the SKILL.md inside it. The whole bundle
is copied — SKILL.md plus references/, scripts/ and assets/ — after validating
the two things that decide whether the skill is usable at all: a kebab-case name
and a non-empty description (the trigger the agent matches a task against).

Scopes, highest priority first — when the same name exists in two scopes, the
higher one wins and the lower one never loads:

  enterprise  machine-wide managed skills (needs admin rights)
  personal    your home directory — available in every repo
  project     checked into this repository, shared with the team
  plugin      installed plugins (read-only, cannot be installed into)

So a personal "code-review" is shadowed by an enterprise "code-review", and
shadows a project one. Use descriptive names (backend-review, not review) to
avoid the collision entirely; this command warns when it detects one.

Examples:
  ctx3 add-skill ./my-skill --scope personal     # available in every repo
  ctx3 add-skill ./my-skill --scope project      # commit it with the code
  ctx3 add-skill ./my-skill/SKILL.md --scope personal
  ctx3 add-skill ./review --name backend-review  # rename to dodge a collision
  ctx3 add-skill ./my-skill --dry-run            # validate + show where it lands`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		src, err := skillwriter.Load(args[0])
		if err != nil {
			return err
		}

		name := addSkillName
		if name == "" {
			name = src.Meta.Name
		}

		tgt, skillsDir, err := resolveSkillDir(skillAs, addSkillRoot)
		if err != nil {
			return err
		}

		if addSkillDry {
			fmt.Printf("skill:       %s\n", name)
			fmt.Printf("description: %s\n", src.Meta.Description)
			fmt.Printf("files:       %d (%s)\n", len(src.Files), strings.Join(src.Files, ", "))
			fmt.Printf("would write: %s/%s  [%s scope]\n", skillsDir, name, skillScope)
			reportShadowing(tgt, name)
			return nil
		}

		dir, files, err := skillwriter.Install(skillwriter.Config{SkillsDir: skillsDir, Force: skillForce}, src, name)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "%s Installed skill %q (%d files) to %s [%s scope]\n", glyph("✓", "+"), name, len(files), dir, skillScope)
		fmt.Fprintf(os.Stderr, "  Restart %s to pick it up.\n", tgt.Name)
		reportShadowing(tgt, name)
		return nil
	},
}

// reportShadowing prints every other scope holding this name and says which
// copy actually loads.
func reportShadowing(tgt target.Target, name string) {
	sc, ok := target.GetScope(skillScope)
	if !ok {
		return
	}
	for _, sd := range target.AllSkillDirs(tgt, addSkillRoot) {
		if sd.Scope.Name == sc.Name {
			continue
		}
		entries, err := listScopeSkills(sd)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Name != name {
				continue
			}
			if sd.Scope.Priority < sc.Priority {
				fmt.Fprintf(os.Stderr, "%s %q also exists in the %s scope (%s) — that one wins, this copy will not load. Rename with --name.\n", glyph("⚠", "!"),
					name, sd.Scope.Name, e.Dir)
			} else {
				fmt.Fprintf(os.Stderr, "ℹ %q also exists in the %s scope (%s) — this copy takes priority over it.\n",
					name, sd.Scope.Name, e.Dir)
			}
		}
	}
}

func init() {
	f := addSkillCmd.Flags()
	f.StringVar(&addSkillName, "name", "", "Install under this name instead of the frontmatter name")
	f.BoolVar(&addSkillDry, "dry-run", false, "Validate and report the destination without writing")
	f.StringVar(&addSkillRoot, "project-root", ".", "Repository root, used by --scope project")
	bindSkillInstallFlags(f)
	rootCmd.AddCommand(addSkillCmd)
}
