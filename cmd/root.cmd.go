package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/internal/mascot"
	"github.com/parsabordbar/ctx3/internal/version"
	"github.com/spf13/cobra"
)

// banner returns the mini skull with ctx3's name, tagline and cwd alongside.
func banner(color bool) string {
	name := lipgloss.NewStyle().Bold(true)
	ver := lipgloss.NewStyle().Faint(true)
	tag := lipgloss.NewStyle().Foreground(lipgloss.Color("#43B5E6"))
	cwd := lipgloss.NewStyle().Faint(true)
	if !color {
		ver, tag, cwd = lipgloss.NewStyle(), lipgloss.NewStyle(), lipgloss.NewStyle()
	}

	text := lipgloss.JoinVertical(lipgloss.Left,
		name.Render("ctx3")+" "+ver.Render(version.Version()),
		tag.Render("Context Tree — read a repo the way an LLM needs it"),
		cwd.Render("tree · context · call graph · one packed file"),
		cwd.Render(prettyCwd()),
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	if color {
		box = box.BorderForeground(lipgloss.Color("#43B5E6"))
	}
	return mascot.Beside(box.Render(text), 3, color)
}

// prettyCwd is the working directory with $HOME collapsed to ~.
func prettyCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wd, home) {
		wd = "~" + wd[len(home):]
	}
	return wd
}

var rootCmd = &cobra.Command{
	Use:           "ctx3",
	Version:       version.Version(),
	Short:         "ctx3 is a CLI tool to Help you and your favorite LLM understand projects faster!",
	Long:          `ctx3 helps you visualize and analyze project structure for better understanding (and LLM context).`,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		color := mascot.Colorable(os.Stdout)
		// Piped/non-interactive, or --plain: just print the static home screen.
		if plainHome || !interactiveTTY() {
			printHome(color)
			return
		}
		chosen := runLauncher(color)
		if chosen == "" {
			return
		}
		root := cmd.Root()
		root.SetArgs([]string{chosen})
		_ = root.Execute()
	},
}

// plainHome forces the static (non-interactive) home screen.
var plainHome bool

// interactiveTTY reports whether both stdin and stdout are terminals.
func interactiveTTY() bool {
	return mascot.Colorable(os.Stdout) && mascot.Colorable(os.Stdin)
}

// commandGroups drives the home screen.
var commandGroups = []struct {
	title string
	items [][2]string // {name, description}
}{
	{"Analyze", [][2]string{
		{"context", "Project context for LLMs — files, deps, README"},
		{"map", "Symbol index — types, funcs, consts — with file:line"},
		{"flow", "Call graph & code flow"},
		{"impact", "Reverse call graph — what calls a function"},
		{"brief", "Task-scoped context pack under a token budget"},
		{"diff-context", "Changed symbols, their callers, tests to run"},
		{"deps", "Internal dependency chain + cycle detection"},
		{"functions", "Function signatures — args & returns — per file/dir"},
		{"db", "Detected databases + relational schema diagram"},
		{"git", "Repo state, recent commits, files that churn most"},
		{"percentage", "File-type breakdown"},
	}},
	{"Output", [][2]string{
		{"pack", "Pack the repo into one AI-friendly file"},
		{"print", "Print the file tree"},
		{"init", "Scaffold an AGENTS.md / CLAUDE.md"},
	}},
	{"Agents", [][2]string{
		{"skills", "Emit the fact-skill bundle (--list to see every scope)"},
		{"add-skill", "Install a hand-written skill into a scope"},
		{"mcp", "Serve the analysis as live MCP tools over stdio"},
	}},
	{"Manage", [][2]string{
		{"version", "Print the ctx3 version"},
		{"update", "Update to the latest release"},
	}},
}

// ANSI styles for the home screen, applied only when color is on.
const (
	ansiReset = "\x1b[0m"
	ansiBlue  = "\x1b[38;2;67;181;230m"
	ansiDim   = "\x1b[2m"
	ansiBold  = "\x1b[1m"
)

func paint(s, code string, color bool) string {
	if !color {
		return s
	}
	return code + s + ansiReset
}

// printHome renders the banner plus the grouped command list.
func printHome(color bool) {
	fmt.Println(banner(color))
	for _, g := range commandGroups {
		fmt.Printf("  %s\n", paint(g.title, ansiBold, color))
		for _, it := range g.items {
			name := paint(fmt.Sprintf("%-11s", it[0]), ansiBlue, color)
			fmt.Printf("    %s %s\n", name, paint(it[1], ansiDim, color))
		}
		fmt.Println()
	}
	fmt.Printf("  %s\n\n", paint("Run  ctx3 <command> --help  for details.", ansiDim, color))
}

func init() {
	contextCmd.Flags().BoolVarP(&analyzer.OutputJSON, "json", "j", false, "Output as JSON")
	contextCmd.Flags().BoolVarP(&analyzer.OutputTOON, "toon", "t", false, "Output as TOON")
	rootCmd.Flags().BoolVar(&plainHome, "plain", false, "Print the static home screen instead of the interactive menu")
	rootCmd.PersistentFlags().BoolVar(&quietTokens, "no-tokens", false, "Suppress the token estimate printed on stderr")
	rootCmd.PersistentFlags().StringVar(&colorMode, "color", "auto", "Color text output: auto|always|never (auto = only on a terminal; NO_COLOR honored)")
	rootCmd.PersistentFlags().BoolVar(&noEmoji, "no-emoji", false, "Use plain ASCII markers instead of emoji and symbols in text output")
	rootCmd.AddCommand(contextCmd)
	rootCmd.AddCommand(percentageCmd)
}

// dirArgCommands take a path positional, so completion offers directories.
var dirArgCommands = map[string]bool{
	"context": true, "percentage": true, "print": true,
	"pack": true, "flow": true, "deps": true, "init": true, "db": true,
	"functions": true, "map": true, "skills": true, "add-skill": true,
	"git": true,
}

func Execute() {
	// Complete directories for path-taking commands (e.g. `ctx3 pack <Tab>`).
	for _, c := range rootCmd.Commands() {
		if dirArgCommands[c.Name()] && c.ValidArgsFunction == nil {
			c.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				return nil, cobra.ShellCompDirectiveFilterDirs
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
