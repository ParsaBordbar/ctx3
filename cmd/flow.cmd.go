package cmd

import (
	"fmt"

	"github.com/parsabordbar/ctx3/flow"
	"github.com/spf13/cobra"
)

var (
	flowBudget     int
	flowMermaid    bool
	flowPackages   bool
	flowOutputPath string
	flowEntryOnly  bool
	flowDepth      int
)

var flowCmd = &cobra.Command{
	Use:   "flow [directory]",
	Short: "Analyze and visualize code flow / call graph",
	Long: `Analyze the call graph and code flow of a Go project.

Type-checks the module for precise call resolution. When the tree does not
compile — mid-refactor, partial checkout — it falls back to parse-only analysis
and marks the result degraded: calls resolve by name, so a method called on a
value is linked only when one type in the module declares it.

Views:
  - Default: tree of function calls from each entry point
  - Packages (-p): the graph collapsed to package granularity — who calls whom,
    weighted by call sites. The function tree is unreadable past a few hundred
    functions; this stays one screen and answers how the system fans out.

Output formats:
  - Text (default), or Mermaid (-m) markdown; combine -p -m for a package diagram
  - -o writes to a file instead of stdout

Examples:
  ctx3 flow .
  ctx3 flow . --packages               # package-level map
  ctx3 flow . --packages --mermaid     # package diagram that actually renders
  ctx3 flow . --mermaid -o flow.md
  ctx3 flow . --entry-only --depth 3
  ctx3 flow . --skill                  # write a Claude Code skill
  ctx3 flow . --skill -o -             # preview SKILL.md, write nothing`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dirArg(args)

		cfg := flow.Config{
			RootDir:   dir,
			EntryOnly: flowEntryOnly,
			MaxDepth:  flowDepth,
		}

		graph, err := flow.AnalyzeFlow(cfg)
		if err != nil {
			return fmt.Errorf("flow analysis failed: %w", err)
		}

		if skillEmit {
			return emitSkill("flow", graph.Skill(projectBaseName(dir)), dir, flowOutputPath)
		}

		var output string
		switch {
		case flowPackages && flowMermaid:
			output = flow.RenderPackageMermaid(flow.PackageGraph(graph, projectBaseName(dir)))
		case flowPackages:
			output = flow.RenderPackageFlow(flow.PackageGraph(graph, projectBaseName(dir)))
		case flowMermaid:
			output = flow.RenderMermaid(graph)
		default:
			output = flow.RenderText(graph)
		}

		return writeOut(fitBudget(output, flowBudget), flowOutputPath, "Flow")
	},
}

func init() {
	flowCmd.Flags().BoolVarP(&flowMermaid, "mermaid", "m", false, "Output as Mermaid flowchart markdown")
	flowCmd.Flags().BoolVarP(&flowPackages, "packages", "p", false, "Collapse the graph to package granularity (one-screen fan-out map)")
	flowCmd.Flags().StringVarP(&flowOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	flowCmd.Flags().BoolVar(&flowEntryOnly, "entry-only", false, "Only trace calls from entry point files (main.go, etc.)")
	flowCmd.Flags().IntVar(&flowDepth, "depth", 0, "Maximum call depth to trace (0 = unlimited)")
	flowCmd.Flags().IntVar(&flowBudget, "budget", 0, "Token budget: truncate the rendering to fit (0 = unlimited)")
	bindSkillEmitFlags(flowCmd.Flags())
	rootCmd.AddCommand(flowCmd)
}
