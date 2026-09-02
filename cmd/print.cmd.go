package cmd

import (
	"github.com/parsabordbar/ctx3/filetree"
	"github.com/spf13/cobra"
)

var (
	printDepth  int
	printSizes  bool
	printOutput string
)

var printCmd = &cobra.Command{
	Use:   "print [directory]",
	Short: "Print a directory tree",
	Long: `Print the project's directory tree, skipping the usual noise
(node_modules, .git, virtualenvs, build output).

Examples:
  ctx3 print .
  ctx3 print . --depth 2       # top two levels only
  ctx3 print . --no-sizes      # names only
  ctx3 print . -o tree.txt`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := "┌── " + glyph("📂 ", "") + "Project structure:\n" + filetree.Render(filetree.Config{
			Root:     dirArg(args),
			MaxDepth: printDepth,
			Sizes:    printSizes,
		})
		return writeOut(out, printOutput, "File tree")
	},
}

func init() {
	f := printCmd.Flags()
	f.IntVar(&printDepth, "depth", 0, "Limit tree depth (0 = unlimited)")
	f.BoolVar(&printSizes, "sizes", true, "Show file sizes")
	f.StringVarP(&printOutput, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(printCmd)
}
