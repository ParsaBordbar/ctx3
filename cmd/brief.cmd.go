package cmd

import (
	"github.com/parsabordbar/ctx3/brief"
	"github.com/spf13/cobra"
)

var (
	briefDir        string
	briefBudget     int
	briefMax        int
	briefDepth      int
	briefSnippet    int
	briefJSON       bool
	briefTOON       bool
	briefOutputPath string
)

var briefCmd = &cobra.Command{
	Use:     "brief <symbol or task>",
	Aliases: []string{"task"},
	Short:   "Task-scoped context pack — the symbols, source and callers one change needs",
	Long: `Build the context for one task instead of the whole repo. The query is a
symbol name ("Scan", "Graph.Skill") or a free-text task ("where do we validate
skill names"); ctx3 ranks every declaration against it, then attaches for each
hit the source of the declaration, what it calls, what calls it, the entry
points it reaches, and what its package imports.

Ranking is deterministic: exact name match first, then name / doc / signature /
path hits on the query's terms (CamelCase is split, stop words dropped).

The result is bounded by --budget: source snippets shrink first, then the
lowest-ranked hits drop, so the output always fits and the top match is never
the part that goes.

Examples:
  ctx3 brief Scan                              # one symbol, everything around it
  ctx3 brief "validate skill names"            # a task, best-matching symbols
  ctx3 brief Write --budget 1500               # fit a small context window
  ctx3 brief Impacted --max 3 --depth 2 -C ./flow
  ctx3 brief Scan -t                           # TOON for an agent`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := brief.Build(brief.Config{
			RootDir: briefDir, Query: args[0], Budget: briefBudget,
			MaxSymbols: briefMax, Depth: briefDepth, Snippet: briefSnippet,
		})
		if err != nil {
			return err
		}
		var output string
		switch {
		case briefJSON, briefTOON:
			output, err = encodeStructured(b, briefTOON)
			if err != nil {
				return err
			}
		default:
			st, err := outStyle(briefOutputPath)
			if err != nil {
				return err
			}
			output = brief.RenderStyled(b, st)
		}
		return writeOut(output, briefOutputPath, "Brief")
	},
}

func init() {
	f := briefCmd.Flags()
	f.StringVarP(&briefDir, "dir", "C", ".", "Directory to analyze")
	f.IntVar(&briefBudget, "budget", 4000, "Token budget for the output (0 = unlimited)")
	f.IntVar(&briefMax, "max", 8, "Maximum symbols to include")
	f.IntVar(&briefDepth, "depth", 3, "Caller levels to walk (0 = unlimited)")
	f.IntVar(&briefSnippet, "snippet", 40, "Maximum source lines per symbol")
	f.BoolVarP(&briefJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&briefTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.StringVarP(&briefOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(briefCmd)
}
