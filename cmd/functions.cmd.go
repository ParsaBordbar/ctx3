package cmd

import (
	"fmt"
	"regexp"

	"github.com/parsabordbar/ctx3/funcs"

	"github.com/spf13/cobra"
)

var (
	funcsRecursive bool
	funcsExported  bool
	funcsTests     bool
	funcsDocs      bool
	funcsGrep      bool
	funcsMarkdown  bool
	funcsJSON      bool
	funcsTOON      bool
	funcsSortName  bool
	funcsMatch     string
	funcsOutput    string
)

var functionsCmd = &cobra.Command{
	Use:     "functions [file|directory]",
	Aliases: []string{"funcs", "fn"},
	Short:   "List function signatures — args and returns — in a file or directory",
	Long: `List every function and method declared in a Go file or directory, with its
receiver, arguments and return types.

Go only — the parameter and result breakdown needs a real parser. For the
one-line-per-symbol index across other languages, use "ctx3 map --lang".

Syntax-only: no build, no type-check, so it works on a single file, a partial
checkout, or code that doesn't currently compile.

Output formats:
  - Default: grouped by file, with line numbers
  - Grep (-g): one "file:line: func ..." line per function, for pipes and editors
  - Markdown (--md), JSON (-j), TOON (-t)

Examples:
  ctx3 functions pack/walker.go            # one file
  ctx3 functions ./deps                    # one package
  ctx3 functions . -r                      # whole repo
  ctx3 functions . -r -e                   # exported API only
  ctx3 functions . -r -m '^New' -g         # constructors, grep-style
  ctx3 functions ./flow -d                 # include doc one-liners`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := dirArg(args)

		cfg := funcs.Config{
			Path:         path,
			Recursive:    funcsRecursive,
			ExportedOnly: funcsExported,
			IncludeTests: funcsTests,
			SortByName:   funcsSortName,
		}
		if funcsMatch != "" {
			re, err := regexp.Compile(funcsMatch)
			if err != nil {
				return fmt.Errorf("invalid --match pattern: %w", err)
			}
			cfg.Match = re
		}

		res, err := funcs.Scan(cfg)
		if err != nil {
			return err
		}

		var output string
		switch {
		case funcsJSON, funcsTOON:
			output, err = encodeStructured(res, funcsTOON)
			if err != nil {
				return err
			}
		case funcsMarkdown:
			output = funcs.RenderMarkdown(res)
		case funcsGrep:
			output = funcs.RenderGrep(res)
		default:
			output = funcs.RenderText(res, funcsDocs)
		}

		return writeOut(output, funcsOutput, "Function list")
	},
}

func init() {
	f := functionsCmd.Flags()
	f.BoolVarP(&funcsRecursive, "recursive", "r", false, "Recurse into subdirectories")
	f.BoolVarP(&funcsExported, "exported", "e", false, "Only exported functions")
	f.BoolVar(&funcsTests, "tests", false, "Include _test.go files")
	f.BoolVarP(&funcsDocs, "docs", "d", false, "Show the first doc-comment line under each signature")
	f.BoolVarP(&funcsGrep, "grep", "g", false, "One file:line: signature per line")
	f.BoolVar(&funcsMarkdown, "md", false, "Output as a Markdown table")
	f.BoolVarP(&funcsJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&funcsTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.BoolVar(&funcsSortName, "sort-name", false, "Sort by function name instead of file order")
	f.StringVarP(&funcsMatch, "match", "m", "", "Only functions whose name matches this regexp")
	f.StringVarP(&funcsOutput, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(functionsCmd)
}
