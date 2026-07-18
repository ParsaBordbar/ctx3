package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/parsabordbar/ctx3/flow"
	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/target"
	"github.com/spf13/cobra"
)

var (
	flowMermaid    bool
	flowOutputPath string
	flowEntryOnly  bool
	flowDepth      int
	flowSkill      bool
	flowAs         string
	flowForce      bool
)

var flowCmd = &cobra.Command{
	Use:   "flow [directory]",
	Short: "Analyze and visualize code flow / call graph",
	Long: `Analyze the call graph and code flow of a Go project.

Output formats:
  - Default: Human-readable tree of function calls
  - Mermaid (-m): Flowchart in Mermaid markdown format (saves to file with -o)

Examples:
  ctx3 flow .
  ctx3 flow . --mermaid
  ctx3 flow . --mermaid -o flow.md
  ctx3 flow . --entry-only --depth 3
  ctx3 flow . --skill                  # write a Claude Code skill
  ctx3 flow . --skill -o -             # preview SKILL.md, write nothing`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		cfg := flow.Config{
			RootDir:   dir,
			EntryOnly: flowEntryOnly,
			MaxDepth:  flowDepth,
		}

		graph, err := flow.AnalyzeFlow(cfg)
		if err != nil {
			return fmt.Errorf("flow analysis failed: %w", err)
		}

		if flowSkill {
			return emitFlowSkill(graph, dir)
		}

		var output string
		if flowMermaid {
			output = flow.RenderMermaid(graph)
		} else {
			output = flow.RenderText(graph)
		}

		if flowOutputPath != "" {
			if err := os.WriteFile(flowOutputPath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Flow written to %s\n", flowOutputPath)
		} else {
			fmt.Print(output)
		}

		return nil
	},
}

// emitFlowSkill writes the call-graph skill, or prints SKILL.md when -o - is used.
func emitFlowSkill(graph *flow.CallGraph, dir string) error {
	sel := flowAs
	if sel == "" {
		sel = "claude" // skills are a Claude Code convention
	}
	tgt, ok := target.Get(sel)
	if !ok {
		return fmt.Errorf("unknown --as value %q (want: %v)", flowAs, target.Names())
	}
	if !tgt.SupportsSkills() {
		return fmt.Errorf("target %q does not support skills (skills are a Claude Code convention)", sel)
	}

	skill := graph.Skill(projectBaseName(dir))

	if flowOutputPath == "-" {
		fmt.Print(skillwriter.RenderSkillMD(skill))
		return nil
	}

	skillDir, files, err := skillwriter.Write(skillwriter.Config{SkillsDir: tgt.SkillDir, Force: flowForce}, skill)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote skill %q (%d files) to %s\n", skill.Name, len(files), skillDir)
	fmt.Fprintf(os.Stderr, "  %s will load it when a task matches its description. Re-run with --force to refresh.\n", tgt.Name)
	return nil
}

// projectBaseName returns the go.mod module base, else the directory base name.
func projectBaseName(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "module ") {
				mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
				if i := strings.LastIndex(mod, "/"); i >= 0 {
					return mod[i+1:]
				}
				return mod
			}
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Base(abs)
	}
	return "project"
}

func init() {
	flowCmd.Flags().BoolVarP(&flowMermaid, "mermaid", "m", false, "Output as Mermaid flowchart markdown")
	flowCmd.Flags().StringVarP(&flowOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	flowCmd.Flags().BoolVar(&flowEntryOnly, "entry-only", false, "Only trace calls from entry point files (main.go, etc.)")
	flowCmd.Flags().IntVar(&flowDepth, "depth", 0, "Maximum call depth to trace (0 = unlimited)")
	flowCmd.Flags().BoolVar(&flowSkill, "skill", false, "Emit a coding-agent skill (SKILL.md + reference files) instead of printing; use -o - to preview")
	flowCmd.Flags().StringVar(&flowAs, "as", "", "target tool for --skill (default: claude)")
	flowCmd.Flags().BoolVar(&flowForce, "force", false, "Overwrite an existing skill directory")
	rootCmd.AddCommand(flowCmd)
}