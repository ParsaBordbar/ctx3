package cmd

import (
	"fmt"

	"github.com/parsabordbar/ctx3/flow"

	"github.com/spf13/cobra"
)

var (
	impactDir     string
	impactDepth   int
	impactMermaid bool
	impactJSON    bool
	impactTOON    bool
	impactOutput  string
)

var impactCmd = &cobra.Command{
	Use:     "impact <symbol>",
	Aliases: []string{"callers", "who-calls"},
	Short:   "Show what transitively calls a function — the blast radius of a change",
	Long: `Walk the call graph backwards from a function and report everything that
reaches it: direct callers, transitive callers, the packages involved, and the
entry points the change can surface at.

The symbol is matched against the graph as an exact "pkg.Func" / "pkg.Recv.Method"
key first, then by bare function or method name, then as a substring. Every match
is reported, so an ambiguous name shows all candidates rather than guessing.

Go only. Type-checks the module when it compiles; when it does not, it falls
back to parse-only analysis and says so — treat "no callers" as unproven then.

Examples:
  ctx3 impact Scan                      # who calls any Scan
  ctx3 impact symbols.Scan              # one exact function
  ctx3 impact "Graph.Skill" -C ./flow   # a method, analyzing another directory
  ctx3 impact Scan --depth 2            # two caller levels
  ctx3 impact Scan --mermaid            # reverse call graph as a diagram`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		graph, err := flow.AnalyzeFlow(flow.Config{RootDir: impactDir})
		if err != nil {
			return fmt.Errorf("flow analysis failed: %w", err)
		}

		imp, err := flow.Impacted(graph, args[0], impactDepth)
		if err != nil {
			return err
		}

		var output string
		switch {
		case impactJSON, impactTOON:
			output, err = encodeStructured(imp, impactTOON)
			if err != nil {
				return err
			}
		case impactMermaid:
			output = flow.RenderImpactMermaid(imp)
		default:
			output = flow.RenderImpactText(imp)
		}

		return writeOut(output, impactOutput, "Impact report")
	},
}

func init() {
	f := impactCmd.Flags()
	f.StringVarP(&impactDir, "dir", "C", ".", "Directory to analyze")
	f.IntVar(&impactDepth, "depth", 0, "Caller levels to walk (0 = unlimited)")
	f.BoolVarP(&impactMermaid, "mermaid", "m", false, "Output as a Mermaid diagram")
	f.BoolVarP(&impactJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&impactTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.StringVarP(&impactOutput, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(impactCmd)
}
