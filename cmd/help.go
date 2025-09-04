package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

var helpCmd = &cobra.Command{
	Use:   "help",
	Short: "Show help for FileTree commands",
	Long:  `Display detailed help information for FileTree commands and usage.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("ctx3 is a CLI tool to analyze project structure.")
		fmt.Println("Available commands:")
		fmt.Println("context [directory]   Analyze project context for LLMs")
		fmt.Println("print [directory]     Print the file tree of the specified directory")
	},
}

func init() {
	rootCmd.AddCommand(helpCmd)
}