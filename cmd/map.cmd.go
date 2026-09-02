package cmd

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/parsabordbar/ctx3/internal/tokens"
	"github.com/parsabordbar/ctx3/symbols"

	"github.com/spf13/cobra"
)

var (
	mapBudget     int
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
	mapLangs      string
	mapOutputPath string
)

var mapCmd = &cobra.Command{
	Use:     "map [file|directory]",
	Aliases: []string{"symbols", "index"},
	Short:   "Index every top-level symbol — types, funcs, consts, vars — with file:line",
	Long: `Build a symbol index: every type, struct, interface, class, function,
method, const and var, with its signature and file:line.

Syntax-only: no build, no type-check, so it works on a partial checkout or code
that doesn't currently compile. Exported symbols only by default.

The index is the cheap alternative to reading files: an agent greps it to find
where something lives instead of opening a directory at a time.

Languages: Go (full parser) plus ` + langList() + ` (pattern-matched,
so an unusually written declaration can be missed). Filter with --lang.

Output formats:
  - Default: grouped by package, then by kind
  - Grep (-g): one "file:line: signature" per symbol
  - Markdown (--md), JSON (-j), TOON (-t)

Examples:
  ctx3 map .                          # exported API of the whole repo
  ctx3 map . -a                       # include unexported
  ctx3 map ./db --members             # struct fields and interface methods
  ctx3 map . --kind struct,interface  # data model only
  ctx3 map . --lang python,typescript # one language of a polyglot repo
  ctx3 map . -m 'Skill' -g            # grep-style, name filter`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := dirArg(args)

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
		langs, err := parseLangs(mapLangs)
		if err != nil {
			return err
		}
		cfg.Langs = langs
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

		if skillEmit {
			return emitSkill("map", idx.Skill(projectBaseName(path)), path, mapOutputPath)
		}

		render := func() (string, error) {
			switch {
			case mapJSON, mapTOON:
				return encodeStructured(idx, mapTOON)
			case mapMarkdown:
				return symbols.RenderMarkdown(idx), nil
			case mapGrep:
				return symbols.RenderGrep(idx), nil
			default:
				return symbols.RenderText(idx, mapDocs), nil
			}
		}
		output, err := render()
		if err != nil {
			return err
		}
		if mapBudget > 0 {
			total := len(idx.Symbols)
			for est := tokens.Estimate(output); est > mapBudget && len(idx.Symbols) > 0; est = tokens.Estimate(output) {
				keep := len(idx.Symbols) * mapBudget / est
				if keep >= len(idx.Symbols) {
					keep = len(idx.Symbols) - 1
				}
				idx.Symbols = idx.Symbols[:keep]
				if output, err = render(); err != nil {
					return err
				}
			}
			if dropped := total - len(idx.Symbols); dropped > 0 {
				fmt.Fprintf(os.Stderr, "trimmed %d symbols to fit %s tokens\n", dropped, tokens.Format(mapBudget))
			}
		}

		return writeOut(output, mapOutputPath, "Symbol map")
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

// parseLangs turns "python,go" into a language filter, rejecting anything the
// scanner cannot read.
func parseLangs(list string) ([]string, error) {
	if strings.TrimSpace(list) == "" {
		return nil, nil
	}
	var out []string
	for raw := range strings.SplitSeq(list, ",") {
		name := strings.ToLower(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if !slices.Contains(symbols.Languages(), name) {
			return nil, fmt.Errorf("unknown language %q (valid: %s)", name, langList())
		}
		out = append(out, name)
	}
	return out, nil
}

func langList() string {
	return strings.Join(symbols.Languages(), ", ")
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
	f.StringVar(&mapLangs, "lang", "", "Comma-separated languages to keep ("+langList()+")")
	f.StringVarP(&mapOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	f.IntVar(&mapBudget, "budget", 0, "Token budget: drop trailing symbols until the output fits (0 = unlimited)")
	bindSkillEmitFlags(f)
	rootCmd.AddCommand(mapCmd)
}
