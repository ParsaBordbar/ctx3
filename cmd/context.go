package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"github.com/parsabordbar/ctx3/analyzer"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context [directory]",
	Short: "Analyze project context for LLMs",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		ctx := analyzer.AnalyzeProject(dir)

		if analyzer.OutputJSON {
			data, _ := json.MarshalIndent(ctx, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Printf("📂 Project: %s\n", ctx.Root)
			fmt.Printf("Files: %d, Dirs: %d\n", ctx.TotalFiles, ctx.TotalDirs)
			if len(ctx.Dependencies) > 0 {
				fmt.Println("Dependencies:", strings.Join(ctx.Dependencies, ", "))
			}
			if ctx.Readme != "" {
				fmt.Println("\nREADME Preview:\n", ctx.Readme)
			}
		}
	},
}