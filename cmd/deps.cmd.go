package cmd

import (
	"fmt"
	"strings"

	"github.com/parsabordbar/ctx3/deps"

	"github.com/spf13/cobra"
)

var (
	depsMermaid    bool
	depsJSON       bool
	depsTOON       bool
	depsOutputPath string
	depsCyclesOnly bool
	depsQuiet      bool
)

var depsCmd = &cobra.Command{
	Use:   "deps [directory]",
	Short: "Analyze the internal dependency chain of a project",
	Long: `Build the internal package dependency chain of a project: which packages
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
		dir := dirArg(args)

		graph, err := deps.Analyze(deps.Config{RootDir: dir})
		if err != nil {
			return err
		}

		if skillEmit {
			return emitSkill("deps", graph.Skill(), dir, depsOutputPath)
		}

		if depsCyclesOnly {
			if len(graph.Cycles) == 0 {
				if !depsQuiet {
					fmt.Println(glyph("✓", "+") + " No circular imports.")
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
		case depsJSON, depsTOON:
			output, err = encodeStructured(graph, depsTOON)
			if err != nil {
				return err
			}
		case depsMermaid:
			output = deps.RenderMermaid(graph)
		default:
			st, err := outStyle(depsOutputPath)
			if err != nil {
				return err
			}
			output = deps.RenderStyled(graph, st)
		}
		return writeOut(output, depsOutputPath, "Dependency chain")
	},
}

func init() {
	depsCmd.Flags().BoolVarP(&depsMermaid, "mermaid", "m", false, "Output as Mermaid dependency graph")
	depsCmd.Flags().BoolVarP(&depsJSON, "json", "j", false, "Output as JSON")
	depsCmd.Flags().BoolVarP(&depsTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	depsCmd.Flags().StringVarP(&depsOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	depsCmd.Flags().BoolVar(&depsCyclesOnly, "cycles-only", false, "Only report circular imports (non-zero exit if any)")
	depsCmd.Flags().BoolVar(&depsQuiet, "quiet", false, "With --cycles-only: suppress the success line (print only on failure)")
	bindSkillEmitFlags(depsCmd.Flags())
	rootCmd.AddCommand(depsCmd)
}
