package cmd

import (
	"fmt"
	"os"

	"github.com/parsabordbar/ctx3/flow"
	"github.com/spf13/cobra"
)

var (
	flowMermaid    bool
	flowOutputPath string
	flowEntryOnly  bool
	flowDepth      int
)

var flowCmd = &cobra.Command{
	Use:   "flow [directory]",
	Short: "Analyze and visualize code flow / call graph",
	Long: `Analyze the call graph and code flow of a Go project.

Output formats:
  - Default: Human-readable tree of function calls
  - Mermaid (-m): Flowchart in Mermaid markdown format (saves to file with -o)

Examples:
  ctx3 flow .
  ctx3 flow . --mermaid
  ctx3 flow . --mermaid -o flow.md
  ctx3 flow . --entry-only --depth 3`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		cfg := flow.Config{
			RootDir:   dir,
			EntryOnly: flowEntryOnly,
			MaxDepth:  flowDepth,
		}

		graph, err := flow.AnalyzeFlow(cfg)
		if err != nil {
			return fmt.Errorf("flow analysis failed: %w", err)
		}

		var output string
		if flowMermaid {
			output = flow.RenderMermaid(graph)
		} else {
			output = flow.RenderText(graph)
		}

		if flowOutputPath != "" {
			if err := os.WriteFile(flowOutputPath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Flow written to %s\n", flowOutputPath)
		} else {
			fmt.Print(output)
		}

		return nil
	},
}

func init() {
	flowCmd.Flags().BoolVarP(&flowMermaid, "mermaid", "m", false, "Output as Mermaid flowchart markdown")
	flowCmd.Flags().StringVarP(&flowOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	flowCmd.Flags().BoolVar(&flowEntryOnly, "entry-only", false, "Only trace calls from entry point files (main.go, etc.)")
	flowCmd.Flags().IntVar(&flowDepth, "depth", 0, "Maximum call depth to trace (0 = unlimited)")
	rootCmd.AddCommand(flowCmd)
}