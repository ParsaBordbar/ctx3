package cmd

import (
	"github.com/parsabordbar/ctx3/db"

	"github.com/spf13/cobra"
)

var (
	dbMermaid     bool
	dbJSON        bool
	dbTOON        bool
	dbOutputPath  string
	dbEnginesOnly bool
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
		dir := dirArg(args)

		report, err := db.Analyze(db.Config{RootDir: dir})
		if err != nil {
			return err
		}

		if skillEmit {
			return emitSkill("db", report.Skill(projectBaseName(dir)), dir, dbOutputPath)
		}
		if dbEnginesOnly {
			report.Schema = nil
		}

		var output string
		switch {
		case dbJSON, dbTOON:
			output, err = encodeStructured(report, dbTOON)
			if err != nil {
				return err
			}
		case dbMermaid:
			output = db.RenderMermaid(report)
		default:
			output = db.RenderText(report)
		}

		return writeOut(output, dbOutputPath, "Database report")
	},
}

func init() {
	dbCmd.Flags().BoolVarP(&dbMermaid, "mermaid", "m", false, "Output as a Mermaid ER diagram")
	dbCmd.Flags().BoolVarP(&dbJSON, "json", "j", false, "Output as JSON")
	dbCmd.Flags().BoolVarP(&dbTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	dbCmd.Flags().StringVarP(&dbOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	dbCmd.Flags().BoolVar(&dbEnginesOnly, "engines-only", false, "Only list detected databases, skip the schema")
	bindSkillEmitFlags(dbCmd.Flags())
	rootCmd.AddCommand(dbCmd)
}
