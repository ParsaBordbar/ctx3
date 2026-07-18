package cmd

import (
	"fmt"
	"os"

	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/internal/version"
	"github.com/spf13/cobra"
)

var icon = `
 ▗▄▄▖ ▗▄▖ ▗▖  ▗▖▗▄▄▄▖▗▄▄▄▖▗▖  ▗▖▗▄▄▄▖    ▗▄▄▄▖▗▄▄▖ ▗▄▄▄▖▗▄▄▄▖
▐▌   ▐▌ ▐▌▐▛▚▖▐▌  █  ▐▌    ▝▚▞▘   █        █  ▐▌ ▐▌▐▌   ▐▌   
▐▌   ▐▌ ▐▌▐▌ ▝▜▌  █  ▐▛▀▀▘  ▐▌    █        █  ▐▛▀▚▖▐▛▀▀▘▐▛▀▀▘
▝▚▄▄▖▝▚▄▞▘▐▌  ▐▌  █  ▐▙▄▄▖▗▞▘▝▚▖  █        █  ▐▌ ▐▌▐▙▄▄▖▐▙▄▄▖
`

var rootCmd = &cobra.Command{
	Use:     "ctx3",
	Version: version.Version(),
	Short:   "ctx3 is a CLI tool to Help you and your favorite LLM understand projects faster!",
	Long:    `ctx3 helps you visualize and analyze project structure for better understanding (and LLM context).`,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(icon)
		fmt.Println("ctx3 is a CLI tool to analyze project structure.")
		fmt.Println("┌── Available commands:")
		fmt.Println("├── context [directory]      Analyze project context for LLMs")
		fmt.Println("├── print [directory]        Print the file tree of the specified directory")
		fmt.Println("├── pack [directory]     	  Pack a repository into a single AI-friendly file")
		fmt.Println("├── flow [directory]         Analyze and visualize code flow / call graph")
		fmt.Println("├── deps [directory]         Analyze the internal dependency chain (Go module)")
		fmt.Println("├── init [directory]         Generate an AGENT.md/CLAUDE.md context file")
		fmt.Println("├── percentage [directory]   Percentage of file types present in the specified directory")
		fmt.Println("├── update                  Update ctx3 to the latest version")
		fmt.Println("└── version                 Print the ctx3 version")
		fmt.Println()
	},
}

func init() {
	contextCmd.Flags().BoolVarP(&analyzer.OutputJSON, "json", "j", false, "Output as JSON")
	contextCmd.Flags().BoolVarP(&analyzer.OutputTOON, "toon", "t", false, "Output as TOON")
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(percentageCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}