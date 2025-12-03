package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/toon-format/toon-go"

	"github.com/spf13/cobra"
)

var contextCmd = &cobra.Command{
	Use:   "context [directory]",
	Short: "Analyze project context for LLMs",
	Long: `Analyze project context and output in different formats.

Output formats:
  - Default: Human-readable text format
  - JSON (-j): Machine-readable JSON format
  - TOON (-t): Token-Oriented Object Notation (compact, LLM-optimized)`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		ctx := analyzer.AnalyzeProject(dir)

		// Handle different output formats
		if analyzer.OutputTOON {
			// Output as TOON format
			encoded, err := toon.Marshal(ctx, toon.WithLengthMarkers(true))
			if err != nil {
				fmt.Printf("Error encoding TOON: %v\n", err)
				return
			}
			fmt.Println(string(encoded))
		} else if analyzer.OutputJSON {
			// Output as JSON
			data, _ := json.MarshalIndent(ctx, "", "  ")
			fmt.Println(string(data))
		} else {
			// Output as human-readable format
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