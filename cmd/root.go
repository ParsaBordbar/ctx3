package cmd

import (
	"fmt"
	"os"
	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "filetree",
	Short: "FileTree is a CLI tool to analyze project structure",
	Long:  `FileTree helps you visualize and analyze project structure for better understanding (and LLM context).`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Use 'filetree print [dir]' to print a file tree")
	},
}

func init() {
	contextCmd.Flags().BoolVarP(&analyzer.OutputJSON, "json", "j", false, "Output as JSON")
	rootCmd.AddCommand(contextCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}