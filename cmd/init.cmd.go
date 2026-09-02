package cmd

import (
	"fmt"
	"os"

	"github.com/parsabordbar/ctx3/agentmd"
	"github.com/parsabordbar/ctx3/target"
	"github.com/spf13/cobra"
)

var (
	initOutputPath string
	initStdout     bool
	initForce      bool
	initAs         string // target selector: agent|claude|gemini|copilot|cursor
)

var initCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Generate an AGENTS.md/CLAUDE.md context file for coding agents",
	Long: `Generate a context file (AGENTS.md by default) that gives coding agents a
head start in this repository. Composes ctx3's analysis — project metadata,
detected build/test/run commands, package/call-graph architecture, language
breakdown and dependencies — into a committable markdown scaffold.

The output is deterministic (no LLM); TODO markers flag what a human or agent
should refine.

Targets (--as): agent -> AGENTS.md, claude -> CLAUDE.md, gemini -> GEMINI.md,
copilot -> .github/copilot-instructions.md.

Examples:
  ctx3 init
  ctx3 init . --as claude
  ctx3 init -o CONTEXT.md --force
  ctx3 init --stdout
  ctx3 init -o -`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true, // runtime errors shouldn't dump the flag list
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := dirArg(args)
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("%s is not a directory", dir)
		}

		out := initOutputPath
		toStdout := initStdout || out == "-"
		// "-" selects stdout; the target's context file still names the document.
		if out == "" || out == "-" {
			sel := initAs
			if sel == "" {
				sel = target.Default
			}
			tgt, ok := target.Get(sel)
			if !ok {
				return fmt.Errorf("unknown --as value %q (want: %v)", initAs, target.Names())
			}
			if tgt.ContextFile == "" {
				return fmt.Errorf("target %q has no single context file; pass -o to choose a path", sel)
			}
			out = tgt.ContextFile
		}

		doc, err := agentmd.Generate(agentmd.Config{RootDir: dir, Title: out})
		if err != nil {
			return err
		}

		if toStdout {
			fmt.Print(doc)
			return nil
		}

		if _, err := os.Stat(out); err == nil && !initForce {
			return fmt.Errorf("%s already exists (use --force to overwrite or --stdout to print)", out)
		}
		if err := os.WriteFile(out, []byte(doc), 0644); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", out)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVarP(&initOutputPath, "output", "o", "", "output file path, or - for stdout (default AGENTS.md)")
	initCmd.Flags().BoolVar(&initStdout, "stdout", false, "print to stdout instead of writing a file")
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite the output file if it exists")
	initCmd.Flags().StringVar(&initAs, "as", "", "target tool: agent|claude|gemini|copilot")
	rootCmd.AddCommand(initCmd)
}
