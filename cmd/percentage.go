package cmd

import (
	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/spf13/cobra"
)

var percentageCmd = &cobra.Command{
	Use:   "percentage",
	Short: "Show file format percentages in the project",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		ctx := analyzer.AnalyzeProject(dir)
		counts := analyzer.CollectFileStats(&ctx)
		filePercentages := analyzer.FilePercentage(counts)
		analyzer.PrettyPrintPercentage(filePercentages)
	},
}
