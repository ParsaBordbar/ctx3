package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/parsabordbar/ctx3/db"
	"github.com/parsabordbar/ctx3/skillwriter"
	"github.com/parsabordbar/ctx3/target"
	toon "github.com/toon-format/toon-go"

	"github.com/spf13/cobra"
)

var (
	dbMermaid     bool
	dbJSON        bool
	dbTOON        bool
	dbOutputPath  string
	dbEnginesOnly bool
	dbSkill       bool
	dbAs          string
	dbForce       bool
)

var dbCmd = &cobra.Command{
	Use:   "db [directory]",
	Short: "Detect the project's databases and diagram the relational schema",
	Long: `Detect every datastore the project uses — from dependency manifests, compose
files, connection strings and committed database files — and, for relational
ones, reconstruct the schema from SQL migrations, schema.sql, schema.prisma and
ORM-tagged Go structs.

Nothing connects to a live database: analysis is static, so the same repo
always produces the same output.

Output formats:
  - Default: engine list + boxed tables with inline foreign-key arrows
  - Mermaid (-m): entity-relationship diagram
  - JSON (-j) / TOON (-t): machine-readable (TOON is compact, LLM-optimized)

Examples:
  ctx3 db .
  ctx3 db . --engines-only
  ctx3 db . --mermaid -o schema.md
  ctx3 db . -t
  ctx3 db . --skill                    # write a Claude Code skill
  ctx3 db . --skill -o -               # preview SKILL.md, write nothing`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		report, err := db.Analyze(db.Config{RootDir: dir})
		if err != nil {
			return err
		}

		if dbSkill {
			return emitDBSkill(report)
		}
		if dbEnginesOnly {
			report.Schema = nil
		}

		var output string
		switch {
		case dbJSON:
			b, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return err
			}
			output = string(b)
		case dbTOON:
			b, err := toon.Marshal(report)
			if err != nil {
				return err
			}
			output = string(b)
		case dbMermaid:
			output = db.RenderMermaid(report)
		default:
			output = db.RenderText(report)
		}

		if dbOutputPath != "" && dbOutputPath != "-" {
			if err := os.WriteFile(dbOutputPath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Database report written to %s\n", dbOutputPath)
			return nil
		}
		fmt.Println(output)
		return nil
	},
}

// emitDBSkill writes the database skill for the selected target.
func emitDBSkill(report *db.Report) error {
	sel := dbAs
	if sel == "" {
		sel = "claude" // skills are a Claude Code convention
	}
	tgt, ok := target.Get(sel)
	if !ok {
		return fmt.Errorf("unknown --as value %q (want: %v)", dbAs, target.Names())
	}
	if !tgt.SupportsSkills() {
		return fmt.Errorf("target %q does not support skills (skills are a Claude Code convention)", sel)
	}

	skill := report.Skill()

	if dbOutputPath == "-" {
		fmt.Print(skillwriter.RenderSkillMD(skill))
		return nil
	}

	dir, files, err := skillwriter.Write(skillwriter.Config{SkillsDir: tgt.SkillDir, Force: dbForce}, skill)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote skill %q (%d files) to %s\n", skill.Name, len(files), dir)
	fmt.Fprintf(os.Stderr, "  %s will load it when a task matches its description. Re-run with --force to refresh.\n", tgt.Name)
	return nil
}

func init() {
	dbCmd.Flags().BoolVarP(&dbMermaid, "mermaid", "m", false, "Output as a Mermaid ER diagram")
	dbCmd.Flags().BoolVarP(&dbJSON, "json", "j", false, "Output as JSON")
	dbCmd.Flags().BoolVarP(&dbTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	dbCmd.Flags().StringVarP(&dbOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	dbCmd.Flags().BoolVar(&dbEnginesOnly, "engines-only", false, "Only list detected databases, skip the schema")
	dbCmd.Flags().BoolVar(&dbSkill, "skill", false, "Emit a coding-agent skill (SKILL.md + reference files) instead of printing; use -o - to preview")
	dbCmd.Flags().StringVar(&dbAs, "as", "", "target tool for --skill (default: claude)")
	dbCmd.Flags().BoolVar(&dbForce, "force", false, "Overwrite an existing skill directory")
	rootCmd.AddCommand(dbCmd)
}
