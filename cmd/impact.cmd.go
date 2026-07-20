package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/parsabordbar/ctx3/flow"
	toon "github.com/toon-format/toon-go"

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

Requires a type-checkable module (same as ` + "`flow`" + `).

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
		case impactJSON:
			b, err := json.MarshalIndent(imp, "", "  ")
			if err != nil {
				return err
			}
			output = string(b)
		case impactTOON:
			b, err := toon.Marshal(imp)
			if err != nil {
				return err
			}
			output = string(b)
		case impactMermaid:
			output = flow.RenderImpactMermaid(imp)
		default:
			output = flow.RenderImpactText(imp)
		}

		if impactOutput != "" && impactOutput != "-" {
			if err := os.WriteFile(impactOutput, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Impact report written to %s\n", impactOutput)
			return nil
		}
		fmt.Println(output)
		return nil
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
