package cmd

import (
	"fmt"

	"github.com/parsabordbar/ctx3/filetree"
	"github.com/spf13/cobra"
)

var printCmd = &cobra.Command{
	Use:   "print [directory]",
	Short: "Print a directory tree",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		fmt.Println("📂 Project structure:")
		filetree.PrintTree(dir, "")
	},
}

func init() {
	rootCmd.AddCommand(printCmd)
}
