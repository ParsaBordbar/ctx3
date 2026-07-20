package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/parsabordbar/ctx3/symbols"
	toon "github.com/toon-format/toon-go"

	"github.com/spf13/cobra"
)

var (
	mapAll        bool
	mapTests      bool
	mapDocs       bool
	mapMembers    bool
	mapNoRecurse  bool
	mapGrep       bool
	mapMarkdown   bool
	mapJSON       bool
	mapTOON       bool
	mapKinds      string
	mapMatch      string
	mapOutputPath string
)

var mapCmd = &cobra.Command{
	Use:     "map [file|directory]",
	Aliases: []string{"symbols", "index"},
	Short:   "Index every top-level symbol — types, funcs, consts, vars — with file:line",
	Long: `Build a symbol index of a Go tree: every type, struct, interface, function,
method, const and var, with its signature and file:line.

Syntax-only: no build, no type-check, so it works on a partial checkout or code
that doesn't currently compile. Exported symbols only by default.

The index is the cheap alternative to reading files: an agent greps it to find
where something lives instead of opening a directory at a time.

Output formats:
  - Default: grouped by package, then by kind
  - Grep (-g): one "file:line: signature" per symbol
  - Markdown (--md), JSON (-j), TOON (-t)

Examples:
  ctx3 map .                          # exported API of the whole repo
  ctx3 map . -a                       # include unexported
  ctx3 map ./db --members             # struct fields and interface methods
  ctx3 map . --kind struct,interface  # data model only
  ctx3 map . -m 'Skill' -g            # grep-style, name filter`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}

		cfg := symbols.Config{
			Path:              path,
			NoRecurse:         mapNoRecurse,
			IncludeUnexported: mapAll,
			IncludeTests:      mapTests,
			Members:           mapMembers,
		}
		kinds, err := parseKinds(mapKinds)
		if err != nil {
			return err
		}
		cfg.Kinds = kinds
		if mapMatch != "" {
			re, err := regexp.Compile(mapMatch)
			if err != nil {
				return fmt.Errorf("invalid --match pattern: %w", err)
			}
			cfg.Match = re
		}

		idx, err := symbols.Scan(cfg)
		if err != nil {
			return err
		}

		var output string
		switch {
		case mapJSON:
			b, err := json.MarshalIndent(idx, "", "  ")
			if err != nil {
				return err
			}
			output = string(b)
		case mapTOON:
			b, err := toon.Marshal(idx)
			if err != nil {
				return err
			}
			output = string(b)
		case mapMarkdown:
			output = symbols.RenderMarkdown(idx)
		case mapGrep:
			output = symbols.RenderGrep(idx)
		default:
			output = symbols.RenderText(idx, mapDocs)
		}

		if mapOutputPath != "" && mapOutputPath != "-" {
			if err := os.WriteFile(mapOutputPath, []byte(output), 0o644); err != nil {
				return fmt.Errorf("writing output: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Symbol map written to %s\n", mapOutputPath)
			return nil
		}
		fmt.Println(output)
		return nil
	},
}

var validKinds = []symbols.Kind{
	symbols.KindFunc, symbols.KindMethod, symbols.KindStruct,
	symbols.KindInterface, symbols.KindType, symbols.KindConst, symbols.KindVar,
}

// parseKinds turns "struct,func" into kinds, rejecting anything unknown.
func parseKinds(list string) ([]symbols.Kind, error) {
	if strings.TrimSpace(list) == "" {
		return nil, nil
	}
	var out []symbols.Kind
	for raw := range strings.SplitSeq(list, ",") {
		name := symbols.Kind(strings.ToLower(strings.TrimSpace(raw)))
		if name == "" {
			continue
		}
		if !slices.Contains(validKinds, name) {
			return nil, fmt.Errorf("unknown kind %q (valid: %s)", name, kindNames())
		}
		out = append(out, name)
	}
	return out, nil
}

func kindNames() string {
	names := make([]string, len(validKinds))
	for i, k := range validKinds {
		names[i] = string(k)
	}
	return strings.Join(names, "|")
}

func init() {
	f := mapCmd.Flags()
	f.BoolVarP(&mapAll, "all", "a", false, "Include unexported symbols")
	f.BoolVar(&mapTests, "tests", false, "Include _test.go files")
	f.BoolVarP(&mapDocs, "docs", "d", false, "Show the first doc-comment line under each symbol")
	f.BoolVar(&mapMembers, "members", false, "Show struct fields and interface methods")
	f.BoolVar(&mapNoRecurse, "no-recurse", false, "Only index the given directory, not subdirectories")
	f.BoolVarP(&mapGrep, "grep", "g", false, "One file:line: signature per line")
	f.BoolVar(&mapMarkdown, "md", false, "Output as Markdown tables")
	f.BoolVarP(&mapJSON, "json", "j", false, "Output as JSON")
	f.BoolVarP(&mapTOON, "toon", "t", false, "Output as TOON (compact, LLM-optimized)")
	f.StringVar(&mapKinds, "kind", "", "Comma-separated kinds to keep ("+kindNames()+")")
	f.StringVarP(&mapMatch, "match", "m", "", "Only symbols whose name matches this regexp")
	f.StringVarP(&mapOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	rootCmd.AddCommand(mapCmd)
}
