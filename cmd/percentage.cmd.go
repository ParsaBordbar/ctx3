package cmd

import (
	"os"

	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/internal/mascot"
	"github.com/spf13/cobra"
)

var percentageOutput string

var percentageCmd = &cobra.Command{
	Use:   "percentage [directory]",
	Short: "Show file format percentages in the project",
	Long: `Break the project down by file type, as a share of total bytes.

Examples:
  ctx3 percentage .
  ctx3 percentage . -o mix.txt`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := analyzer.AnalyzeProject(dirArg(args))
		pcts := analyzer.FilePercentage(analyzer.CollectFileStats(&ctx))

		// Bars are colored only for a terminal; a file or a pipe gets plain text.
		color := percentageOutput == "" && mascot.Colorable(os.Stdout)
		return writeOut(analyzer.RenderPercentage(pcts, color), percentageOutput, "File percentages")
	},
}

func init() {
	percentageCmd.Flags().StringVarP(&percentageOutput, "output", "o", "", "Write output to file (default: stdout)")
}
