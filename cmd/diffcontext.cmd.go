package cmd

import (
	"errors"
	"fmt"

	"github.com/parsabordbar/ctx3/diffctx"
	"github.com/parsabordbar/ctx3/gitfacts"
	"github.com/spf13/cobra"
)

var (
	diffDir        string
	diffBudget     int
	diffDepth      int
	diffJSON       bool
	diffTOON       bool
	diffOutputPath string
)

var diffContextCmd = &cobra.Command{
	Use:     "diff-context [ref]",
	Aliases: []string{"changes", "diff"},
	Short:   "Context for a change — changed symbols, their callers, and the tests that cover them",
	Long: `Describe what a change touches, not what the tree looks like. Diffs the
working tree against ref (default HEAD, so uncommitted and untracked work),
maps every hunk onto the declaration it lands in, then walks the call graph
backwards from each changed function and greps the test files that reference
it.

This is the input a review agent or a CI bot needs: which symbols moved, who
depends on them, which entry points they reach, and which tests to run.

Examples:
  ctx3 diff-context                     # uncommitted work vs HEAD
  ctx3 diff-context main                # this branch's work vs main
  ctx3 diff-context HEAD~3 --budget 2000
  ctx3 diff-context origin/main -t      # TOON for an agent`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := ""
		if len(args) > 0 {
			ref = args[0]
		}
		c, err := diffctx.Build(diffctx.Config{RootDir: diffDir, Ref: ref, Budget: diffBudget, Depth: diffDepth})
		if err != nil {
			if errors.Is(err, gitfacts.ErrNotARepo) {
				return fmt.Errorf("%s is not a git repository", diffDir)
			}
			return err
		}
		var output string
		switch {
		case diffJSON, diffTOON:
			output, err = encodeStructured(c, diffTOON)
			if err != nil {
				return err
			}
		default:
			output = diffctx.RenderText(c)
		}
		return writeOut(output, diffOutputPath, "Change context")
	},
}

func init() {
	f := diffContextCmd.Flags()
	f.StringVarP(&diffDir, "dir", "C", ".", "Directory to analyze")
	f.IntVar(&diffBudget, "budget", 0, "Token budget for the output (0 = unlimited)")
	f.IntVar(&diffDepth, "depth", 3, "Caller levels to walk (0 = unlimited)")
	f.BoolVarP(&diffJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&diffTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.StringVarP(&diffOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(diffContextCmd)
}
