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
  - TOON (-t): Token-Oriented Object Notation (compact, LLM-optimized)

Examples:
  ctx3 context .
  ctx3 context . --skill               # write a Claude Code overview skill`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dirArg(args)

		ctx := analyzer.AnalyzeProject(dir)

		if skillEmit {
			return emitSkill("context", ctx.Skill(projectBaseName(dir)), dir, "")
		}

		// The per-file list dominates the structured payload — name, type, path,
		// size, line count and mtime for every file in the repo — and is rarely
		// what the caller wants from an overview, so it is opt-in. The MCP tool
		// has always trimmed it; the CLI used to dump it.
		encodable := ctx
		if !contextFiles {
			encodable.Files = nil
		}

		switch {
		case analyzer.OutputTOON:
			encoded, err := toon.Marshal(encodable, toon.WithLengthMarkers(true))
			if err != nil {
				return fmt.Errorf("encoding TOON: %w", err)
			}
			fmt.Println(string(encoded))
		case analyzer.OutputJSON:
			data, _ := json.MarshalIndent(encodable, "", "  ")
			fmt.Println(string(data))
		default:
			fmt.Printf("📂 Project: %s\n", ctx.Root)
			fmt.Printf("Files: %d, Dirs: %d\n", ctx.TotalFiles, ctx.TotalDirs)
			if len(ctx.Dependencies) > 0 {
				fmt.Printf("Dependencies (%d direct", len(ctx.Dependencies))
				if ctx.IndirectCount > 0 {
					fmt.Printf(", %d indirect", ctx.IndirectCount)
				}
				fmt.Println("):", strings.Join(ctx.Dependencies, ", "))
			}
			if ctx.Readme != "" {
				fmt.Printf("\nREADME preview:\n%s\n", ctx.Readme)
			}
		}
		return nil
	},
}

// contextFiles opts into the per-file listing in JSON/TOON output.
var contextFiles bool

func init() {
	contextCmd.Flags().BoolVar(&contextFiles, "files", false,
		"Include the per-file listing in JSON/TOON output (large on big repos)")
	bindSkillEmitFlags(contextCmd.Flags())
}
