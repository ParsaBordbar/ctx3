package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/parsabordbar/ctx3/pack"
	"github.com/spf13/cobra"
)

var (
	packOutputPath    string
	packFormat        string // xml|md|txt
	packRespectGit    bool
	packInclude       []string
	packIgnore        []string
	packMaxFileBytes  int64
	packMaxTotalBytes int64
	packBinary        string // skip|hex|base64
	packSort          string // paths|ext
	packSection       string // all|structure|files
	packRedact        []string
	packConcurrency   int
	packCompact       bool // NEW
)

var packCmd = &cobra.Command{
	Use:   "pack [directory]",
	Short: "Pack a repository into a single AI-friendly file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		cfg, err := collectPackConfigFromFlags(root)
		if err != nil {
			return err
		}

		out, report, err := pack.Pack(context.Background(), cfg)
		if err != nil {
			return err
		}

		if cfg.OutputPath != "" {
			if err := os.WriteFile(cfg.OutputPath, out, 0o644); err != nil {
				return err
			}
		} else {
			fmt.Print(string(out))
		}

		fmt.Fprintf(os.Stderr, "Packed %d files (%d skipped), %d bytes\n",
			report.FilesIncluded, report.FilesSkipped, report.TotalBytes)
		for _, w := range report.Warnings {
			fmt.Fprintf(os.Stderr, "warn: %s\n", w)
		}
		return nil
	},
}

func init() {
	packRespectGit = true

	packCmd.Flags().StringVarP(&packOutputPath, "output", "o", "", "Write output to file (default: stdout)")
	packCmd.Flags().StringVarP(&packFormat, "format", "f", "xml", "Output format: xml|md|txt")
	packCmd.Flags().BoolVar(&packRespectGit, "respect-gitignore", true, "Respect .gitignore rules")
	packCmd.Flags().StringSliceVar(&packInclude, "include", nil, "Comma-separated globs to include (applied after ignores)")
	packCmd.Flags().StringSliceVar(&packIgnore, "ignore", nil, "Comma-separated globs to ignore (higher precedence than .gitignore)")
	packCmd.Flags().Int64Var(&packMaxFileBytes, "max-file-bytes", 0, "Skip any single file larger than this many bytes (0 = unlimited)")
	packCmd.Flags().Int64Var(&packMaxTotalBytes, "max-total-bytes", 0, "Stop once accumulated content exceeds this many bytes (0 = unlimited)")
	packCmd.Flags().StringVar(&packBinary, "binary", "skip", "How to handle binary files: skip|hex|base64")
	packCmd.Flags().StringVar(&packSort, "sort", "paths", "Sort order for files: paths|ext")
	packCmd.Flags().StringVar(&packSection, "section", "all", "Which sections to output: all|structure|files")
	packCmd.Flags().StringSliceVar(&packRedact, "redact", nil, "Comma-separated regex patterns to redact from file contents")
	packCmd.Flags().IntVar(&packConcurrency, "concurrency", 0, "Number of concurrent file reads (0 = auto)")
	packCmd.Flags().BoolVar(&packCompact, "compact", false, "Remove extra blank lines between sections and files") // NEW

	rootCmd.AddCommand(packCmd)
}

func collectPackConfigFromFlags(root string) (pack.Config, error) {
	var cfg pack.Config
	cfg.RootDir = root
	cfg.OutputPath = packOutputPath

	switch strings.ToLower(packFormat) {
	case "xml":
		cfg.OutputFormat = pack.FormatXML
	case "md":
		cfg.OutputFormat = pack.FormatMD
	case "txt":
		cfg.OutputFormat = pack.FormatTXT
	default:
		return cfg, fmt.Errorf("invalid --format: %s (expected xml|md|txt)", packFormat)
	}

	switch strings.ToLower(packBinary) {
	case "skip":
		cfg.BinaryHandling = pack.BinarySkip
	case "hex":
		cfg.BinaryHandling = pack.BinaryHex
	case "base64":
		cfg.BinaryHandling = pack.BinaryBase64
	default:
		return cfg, fmt.Errorf("invalid --binary: %s (expected skip|hex|base64)", packBinary)
	}

	switch strings.ToLower(packSort) {
	case "paths":
		cfg.SortByExt = false
	case "ext":
		cfg.SortByExt = true
	default:
		return cfg, fmt.Errorf("invalid --sort: %s (expected paths|ext)", packSort)
	}

	cfg.RespectGitignore = packRespectGit
	cfg.IncludeGlobs = normalizeSlice(packInclude)
	cfg.IgnoreGlobs = normalizeSlice(packIgnore)
	cfg.MaxFileBytes = packMaxFileBytes
	cfg.MaxTotalBytes = packMaxTotalBytes
	cfg.RedactPatterns = normalizeSlice(packRedact)
	cfg.Concurrency = packConcurrency
	cfg.Compact = packCompact // NEW

	switch strings.ToLower(packSection) {
	case "all":
		cfg.Sections.Structure = true
		cfg.Sections.Files = true
	case "structure":
		cfg.Sections.Structure = true
		cfg.Sections.Files = false
	case "files":
		cfg.Sections.Structure = false
		cfg.Sections.Files = true
	default:
		return cfg, errors.New("invalid --section: expected all|structure|files")
	}

	return cfg, nil
}

func normalizeSlice(in []string) []string {
	var out []string
	for _, s := range in {
		for _, piece := range strings.Split(s, ",") {
			p := strings.TrimSpace(piece)
			if p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
