package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/parsabordbar/ctx3/deps"
	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/target"
	toon "github.com/toon-format/toon-go"

	"github.com/spf13/cobra"
)

var (
	depsMermaid    bool
	depsJSON       bool
	depsTOON       bool
	depsOutputPath string
	depsCyclesOnly bool
	depsQuiet      bool
	depsSkill      bool
	depsAs         string
	depsForce      bool
)

var depsCmd = &cobra.Command{
	Use:   "deps [directory]",
	Short: "Analyze the internal dependency chain of a Go module",
	Long: `Build the internal package dependency chain of a Go module: which packages
import which, split from external dependencies, with circular-import detection.

Output formats:
  - Default: human-readable dependency tree
  - Mermaid (-m): dependency graph in Mermaid markdown
  - JSON (-j) / TOON (-t): machine-readable (TOON is compact, LLM-optimized)

Examples:
  ctx3 deps .
  ctx3 deps . --mermaid -o deps.md
  ctx3 deps . --cycles-only            # non-zero exit on a cycle
  ctx3 deps . --cycles-only --quiet    # CI: print only on failure
  ctx3 deps . -t
  ctx3 deps . --skill                  # write a Claude Code skill
  ctx3 deps . --skill -o -             # preview SKILL.md, write nothing`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		graph, err := deps.Analyze(deps.Config{RootDir: dir})
		if err != nil {
			return err
		}

		if depsSkill {
			return emitDepsSkill(graph)
		}

		if depsCyclesOnly {
			if len(graph.Cycles) == 0 {
				if !depsQuiet {
					fmt.Println("✓ No circular imports.")
				}
				return nil
			}
			for _, cyc := range graph.Cycles {
				short := make([]string, len(cyc))
				for i, c := range cyc {
					short[i] = graph.ShortPath(c)
				}
				fmt.Printf("%s → %s\n", strings.Join(short, " → "), short[0])
			}
			return fmt.Errorf("%d circular import(s) detected", len(graph.Cycles))
		}

		var output string
		switch {
		case depsJSON:
			b, err := json.MarshalIndent(graph, "", "  ")
			if err != nil {
				return err
			}
			output = string(b)
		case depsTOON:
			b, err := toon.Marshal(graph)
			if err != nil {
				return err
			}
			output = string(b)
		case depsMermaid:
			output = deps.RenderMermaid(graph)
		default:
			output = deps.RenderText(graph)
		}

		if depsOutputPath != "" {
			if err := os.WriteFile(depsOutputPath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Dependency chain written to %s\n", depsOutputPath)
			return nil
		}
		fmt.Println(output)
		return nil
	},
}

// emitDepsSkill writes the dependency-chain skill for the selected target,
// or prints SKILL.md to stdout when --output - is used.
func emitDepsSkill(graph *deps.Graph) error {
	sel := depsAs
	if sel == "" {
		sel = "claude" // skills are a Claude Code convention
	}
	tgt, ok := target.Get(sel)
	if !ok {
		return fmt.Errorf("unknown --as value %q (want: %v)", depsAs, target.Names())
	}
	if !tgt.SupportsSkills() {
		return fmt.Errorf("target %q does not support skills (skills are a %s convention)", sel, "Claude Code")
	}

	skill := graph.Skill()

	if depsOutputPath == "-" {
		fmt.Print(skillwriter.RenderSkillMD(skill))
		return nil
	}

	dir, files, err := skillwriter.Write(skillwriter.Config{SkillsDir: tgt.SkillDir, Force: depsForce}, skill)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote skill %q (%d files) to %s\n", skill.Name, len(files), dir)
	fmt.Fprintf(os.Stderr, "  %s will load it when a task matches its description. Re-run with --force to refresh.\n", tgt.Name)
	return nil
}

func init() {
	depsCmd.Flags().BoolVarP(&depsMermaid, "mermaid", "m", false, "Output as Mermaid dependency graph")
	depsCmd.Flags().BoolVarP(&depsJSON, "json", "j", false, "Output as JSON")
	depsCmd.Flags().BoolVarP(&depsTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	depsCmd.Flags().StringVarP(&depsOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	depsCmd.Flags().BoolVar(&depsCyclesOnly, "cycles-only", false, "Only report circular imports (non-zero exit if any)")
	depsCmd.Flags().BoolVar(&depsQuiet, "quiet", false, "With --cycles-only: suppress the success line (print only on failure)")
	depsCmd.Flags().BoolVar(&depsSkill, "skill", false, "Emit a coding-agent skill (SKILL.md + reference files) instead of printing; use -o - to preview")
	depsCmd.Flags().StringVar(&depsAs, "as", "", "target tool for --skill (default: claude)")
	depsCmd.Flags().BoolVar(&depsForce, "force", false, "Overwrite an existing skill directory")
	rootCmd.AddCommand(depsCmd)
}
