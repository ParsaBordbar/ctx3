package cmd

import (
	"errors"
	"fmt"

	"github.com/parsabordbar/ctx3/gitfacts"
	"github.com/spf13/cobra"
)

var (
	gitCommits    int
	gitWindow     int
	gitTop        int
	gitMarkdown   bool
	gitJSON       bool
	gitTOON       bool
	gitOutputPath string
)

var gitCmd = &cobra.Command{
	Use:     "git [directory]",
	Aliases: []string{"history", "churn"},
	Short:   "Repository state, recent commits and the files that churn most",
	Long: `Report what the repository's history says about the code: the current branch
and HEAD, what is uncommitted right now, the recent commits, and the files that
change most often.

This is the one thing an agent cannot read off the source: which parts of the
codebase are actually being worked on. The uncommitted list is the working set;
the hot-file list is where effort has been going.

Output formats:
  - Default: compact text
  - Markdown (--md), JSON (-j), TOON (-t)

Examples:
  ctx3 git .
  ctx3 git . --commits 30          # a longer log
  ctx3 git . --window 500 --top 25 # churn over more history
  ctx3 git . --md -o HISTORY.md
  ctx3 git . --skill               # write a Claude Code skill
  ctx3 git . --skill -o -          # preview SKILL.md, write nothing`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dirArg(args)

		rep, err := gitfacts.Analyze(gitfacts.Config{
			RootDir:     dir,
			Commits:     gitCommits,
			ChurnWindow: gitWindow,
			TopFiles:    gitTop,
		})
		if err != nil {
			if errors.Is(err, gitfacts.ErrNotARepo) {
				return fmt.Errorf("%s is not a git repository", dir)
			}
			return err
		}

		if skillEmit {
			return emitSkill("git", rep.Skill(projectBaseName(dir)), dir, gitOutputPath)
		}

		var output string
		switch {
		case gitJSON, gitTOON:
			output, err = encodeStructured(rep, gitTOON)
			if err != nil {
				return err
			}
		case gitMarkdown:
			output = gitfacts.RenderMarkdown(rep)
		default:
			st, err := outStyle(gitOutputPath)
			if err != nil {
				return err
			}
			output = gitfacts.RenderStyled(rep, st)
		}

		return writeOut(output, gitOutputPath, "Repository facts")
	},
}

func init() {
	f := gitCmd.Flags()
	f.IntVar(&gitCommits, "commits", 15, "How many recent commits to list")
	f.IntVar(&gitWindow, "window", 200, "How many commits to measure churn over")
	f.IntVar(&gitTop, "top", 15, "How many hot files to list")
	f.BoolVar(&gitMarkdown, "md", false, "Output as Markdown tables")
	f.BoolVarP(&gitJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&gitTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.StringVarP(&gitOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	bindSkillEmitFlags(f)
	rootCmd.AddCommand(gitCmd)
}
