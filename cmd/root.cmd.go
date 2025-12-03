package cmd

import (
	"fmt"
	"os"
	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ctx3",
	Short: "ctx3 is a CLI tool to analyze project structure",
	Long:  `ctx3 helps you visualize and analyze project structure for better understanding (and LLM context).`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("┌──| ctx3 is a CLI tool to analyze project structure |")
		fmt.Println("├── Available commands:")
		fmt.Println("├── context [directory]   Analyze project context for LLMs")
		fmt.Println("└── print [directory]     Print the file tree of the specified directory")	
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