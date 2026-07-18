// Package agentmd generates an AGENT.md / CLAUDE.md style context file for a
// project by composing ctx3's existing analysis engines (analyzer, flow) with
// build/test/run command detection. Output is a deterministic scaffold meant to
// be committed and then refined by a human or coding agent.
package agentmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/flow"
)

// Config controls generation.
type Config struct {
	RootDir  string
	Title    string // output filename shown in the header note, e.g. "AGENT.md"
	MaxFiles int    // number of largest files to list (0 = default)
}

// CommandGroup is a named block of shell commands (build, test, run, ...).
type CommandGroup struct {
	Title string
	Lines []string // each "command  # comment"
}

// Generate builds the markdown document as a string.
func Generate(cfg Config) (string, error) {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}
	if cfg.Title == "" {
		cfg.Title = "AGENT.md"
	}
	if cfg.MaxFiles == 0 {
		cfg.MaxFiles = 8
	}

	ctx := analyzer.AnalyzeProject(cfg.RootDir)
	name := projectName(cfg.RootDir)

	var sb strings.Builder
	writeHeader(&sb, cfg.Title, name)
	writeOverview(&sb, &ctx)
	writeCommands(&sb, DetectCommands(cfg.RootDir))
	writeArchitecture(&sb, cfg.RootDir, &ctx)
	writeStructure(&sb, cfg.RootDir)
	writeLanguages(&sb, &ctx)
	writeDependencies(&sb, &ctx)
	writeKeyFiles(&sb, &ctx, cfg.MaxFiles)

	return sb.String(), nil
}

// ─── Sections ────────────────────────────────────────────────────────────────

func writeHeader(sb *strings.Builder, title, name string) {
	fmt.Fprintf(sb, "# %s\n\n", title)
	fmt.Fprintf(sb, "This file gives coding agents context to work in the **%s** repository.\n", name)
	sb.WriteString("_Scaffolded by [ctx3](https://github.com/parsabordbar/ctx3) `init` — review and refine the TODO markers._\n\n")
}

func writeOverview(sb *strings.Builder, ctx *analyzer.ProjectContext) {
	sb.WriteString("## Overview\n\n")
	readme := strings.TrimSpace(strings.TrimSuffix(ctx.Readme, "..."))
	if readme != "" {
		// Collapse to a single blurb; drop leading markdown headings.
		blurb := firstProse(readme)
		if blurb != "" {
			fmt.Fprintf(sb, "%s\n\n", blurb)
		}
	}
	fmt.Fprintf(sb, "- Files: **%d** across **%d** %s\n", ctx.TotalFiles, ctx.TotalDirs, plural(ctx.TotalDirs, "directory", "directories"))
	if ep := entryPoints(ctx); len(ep) > 0 {
		fmt.Fprintf(sb, "- Entry point(s): %s\n", strings.Join(codeList(ep), ", "))
	}
	sb.WriteString("\n> TODO: one-paragraph description of what this project does and who uses it.\n\n")
}

func writeCommands(sb *strings.Builder, groups []CommandGroup) {
	sb.WriteString("## Commands\n\n")
	if len(groups) == 0 {
		sb.WriteString("> TODO: no build system detected — document the build/test/run commands.\n\n")
		return
	}
	sb.WriteString("```bash\n")
	for i, g := range groups {
		fmt.Fprintf(sb, "# %s\n", g.Title)
		for _, l := range g.Lines {
			fmt.Fprintf(sb, "%s\n", l)
		}
		if i < len(groups)-1 {
			sb.WriteString("\n")
		}
	}
	sb.WriteString("```\n\n")
}

func writeArchitecture(sb *strings.Builder, root string, ctx *analyzer.ProjectContext) {
	sb.WriteString("## Architecture\n\n")

	// Top-level layout.
	if dirs := topLevelDirs(root); len(dirs) > 0 {
		sb.WriteString("Top-level layout:\n\n")
		for _, d := range dirs {
			fmt.Fprintf(sb, "- `%s/`\n", d)
		}
		sb.WriteString("\n")
	}

	// Go call-graph packages, if any.
	graph, err := flow.AnalyzeFlow(flow.Config{RootDir: root})
	if err == nil && len(graph.Packages) > 0 {
		sb.WriteString("Go packages (from call-graph analysis):\n\n")
		counts := make(map[string]int)
		for _, n := range graph.Nodes {
			counts[n.Package]++
		}
		for _, p := range graph.Packages {
			fmt.Fprintf(sb, "- `%s` — %d function(s). TODO: describe responsibility.\n", p, counts[p])
		}
		if len(graph.Entries) > 0 {
			fmt.Fprintf(sb, "\nExecution starts at: %s\n", strings.Join(codeList(graph.Entries), ", "))
		}
		sb.WriteString("\n> Tip: run `ctx3 flow . --mermaid` for a full call-graph diagram.\n")
	}
	sb.WriteString("\n> TODO: explain the big-picture flow that requires reading multiple files.\n\n")
}

func writeLanguages(sb *strings.Builder, ctx *analyzer.ProjectContext) {
	counts := analyzer.CollectFileStats(ctx)
	pcts := analyzer.FilePercentage(counts)
	if len(pcts) == 0 {
		return
	}
	type kv struct {
		ext string
		pct float64
	}
	var rows []kv
	for e, p := range pcts {
		rows = append(rows, kv{e, p})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].pct > rows[j].pct })

	sb.WriteString("## Languages\n\n")
	shown := 0
	for _, r := range rows {
		if r.pct < 1.0 || shown >= 8 {
			continue
		}
		fmt.Fprintf(sb, "- `%s` — %.1f%%\n", r.ext, r.pct)
		shown++
	}
	sb.WriteString("\n")
}

func writeDependencies(sb *strings.Builder, ctx *analyzer.ProjectContext) {
	if len(ctx.Dependencies) == 0 {
		return
	}
	sb.WriteString("## Dependencies\n\n")
	for _, d := range ctx.Dependencies {
		fmt.Fprintf(sb, "- `%s`\n", d)
	}
	sb.WriteString("\n")
}

// writeKeyFiles lists entry points first, then the largest source files,
// dropping generated/lockfile/vendored noise.
func writeKeyFiles(sb *strings.Builder, ctx *analyzer.ProjectContext, n int) {
	var files []analyzer.FileInfo
	for _, f := range ctx.Files {
		if isNoiseFile(filepath.Base(f.Path)) || inNoiseDir(f.Path) {
			continue
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].IsEntryPoint != files[j].IsEntryPoint {
			return files[i].IsEntryPoint
		}
		return files[i].Lines > files[j].Lines
	})
	sb.WriteString("## Key files\n\n")
	for i, f := range files {
		if i >= n {
			break
		}
		tag := ""
		if f.IsEntryPoint {
			tag = " — entry point"
		}
		fmt.Fprintf(sb, "- `%s` — %d lines%s\n", f.Path, f.Lines, tag)
	}
	sb.WriteString("\n")
}

// writeStructure renders a depth-limited directory tree.
func writeStructure(sb *strings.Builder, root string) {
	var body strings.Builder
	writeTree(&body, root, "", 0)
	if body.Len() == 0 {
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sb.WriteString("## Structure\n\n```\n")
	fmt.Fprintf(sb, "%s/\n", filepath.Base(abs))
	sb.WriteString(body.String())
	sb.WriteString("```\n\n")
}

const (
	treeMaxDepth    = 3
	treeMaxChildren = 40
)

// writeTree walks dir into sb, dirs before files, capped in depth and breadth.
func writeTree(sb *strings.Builder, dir, prefix string, depth int) {
	if depth >= treeMaxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var kept []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() && noiseDirs[name] {
			continue
		}
		if !e.IsDir() && isNoiseFile(name) {
			continue
		}
		kept = append(kept, e)
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].IsDir() != kept[j].IsDir() {
			return kept[i].IsDir() // dirs first
		}
		return kept[i].Name() < kept[j].Name()
	})

	truncated := 0
	if len(kept) > treeMaxChildren {
		truncated = len(kept) - treeMaxChildren
		kept = kept[:treeMaxChildren]
	}
	for i, e := range kept {
		isLast := i == len(kept)-1 && truncated == 0
		branch, childPrefix := "├── ", prefix+"│   "
		if isLast {
			branch, childPrefix = "└── ", prefix+"    "
		}
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		fmt.Fprintf(sb, "%s%s%s\n", prefix, branch, name)
		if e.IsDir() {
			writeTree(sb, filepath.Join(dir, e.Name()), childPrefix, depth+1)
		}
	}
	if truncated > 0 {
		fmt.Fprintf(sb, "%s└── … %d more\n", prefix, truncated)
	}
}

// ─── Command detection ───────────────────────────────────────────────────────

// DetectCommands inspects manifest files at root and returns build/test/run
// command groups for the detected ecosystem(s).
func DetectCommands(root string) []CommandGroup {
	var groups []CommandGroup

	if exists(root, "go.mod") {
		groups = append(groups, CommandGroup{"Go", []string{
			"go build ./...            # build",
			"go test ./...             # run all tests",
			"go test ./pkg -run TestX  # run a single test",
			"go vet ./...              # static checks",
		}})
	}
	if exists(root, "package.json") {
		lines := []string{"npm install               # install deps"}
		for _, s := range npmScripts(root) {
			lines = append(lines, fmt.Sprintf("npm run %-18s# package.json script", s))
		}
		groups = append(groups, CommandGroup{"Node / npm", lines})
	}
	if exists(root, "Cargo.toml") {
		groups = append(groups, CommandGroup{"Rust / Cargo", []string{
			"cargo build               # build",
			"cargo test                # run tests",
			"cargo run                 # run",
		}})
	}
	if exists(root, "pyproject.toml") || exists(root, "requirements.txt") || exists(root, "setup.py") {
		groups = append(groups, CommandGroup{"Python", []string{
			"pip install -r requirements.txt  # install deps",
			"pytest                    # run tests",
		}})
	}
	if targets := makeTargets(root); len(targets) > 0 {
		var lines []string
		for _, t := range targets {
			lines = append(lines, fmt.Sprintf("make %-20s# Makefile target", t))
		}
		groups = append(groups, CommandGroup{"Make", lines})
	}
	return groups
}

// ─── Noise filtering ─────────────────────────────────────────────────────────

var noiseDirs = map[string]bool{
	"node_modules": true, "vendor": true, "target": true, "dist": true,
	"build": true, "out": true, "bin": true, "pkg": true, "coverage": true,
	"venv": true, "__pycache__": true, "site-packages": true,
}

var noiseFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"Cargo.lock": true, "go.sum": true, "poetry.lock": true,
	"composer.lock": true, "Gemfile.lock": true, "Pipfile.lock": true,
}

var noiseSuffixes = []string{".min.js", ".min.css", ".map", ".lock", ".log", ".snap"}

func isNoiseFile(name string) bool {
	if noiseFiles[name] {
		return true
	}
	lower := strings.ToLower(name)
	for _, suf := range noiseSuffixes {
		if strings.HasSuffix(lower, suf) {
			return true
		}
	}
	return false
}

func inNoiseDir(p string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if noiseDirs[seg] {
			return true
		}
	}
	return false
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}

func projectName(root string) string {
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "module ") {
				mod := strings.TrimSpace(strings.TrimPrefix(line, "module "))
				return path_base(mod)
			}
		}
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "project"
	}
	return filepath.Base(abs)
}

func path_base(mod string) string {
	if i := strings.LastIndex(mod, "/"); i >= 0 {
		return mod[i+1:]
	}
	return mod
}

func entryPoints(ctx *analyzer.ProjectContext) []string {
	var out []string
	for _, f := range ctx.Files {
		if f.IsEntryPoint {
			out = append(out, f.Path)
		}
	}
	sort.Strings(out)
	return out
}

func topLevelDirs(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
			continue
		}
		dirs = append(dirs, name)
	}
	sort.Strings(dirs)
	return dirs
}

// npmScripts returns the script names defined in package.json.
func npmScripts(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	var names []string
	for k := range pkg.Scripts {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// makeTargets extracts top-level target names from a Makefile.
func makeTargets(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil
	}
	var targets []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || line[0] == '\t' || line[0] == '#' || line[0] == '.' {
			continue
		}
		i := strings.IndexByte(line, ':')
		if i <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:i])
		if name == "" || strings.ContainsAny(name, " =") || seen[name] {
			continue
		}
		seen[name] = true
		targets = append(targets, name)
	}
	return targets
}

// firstProse returns the first non-heading, non-blank block of README text.
func firstProse(readme string) string {
	for _, line := range strings.Split(readme, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "[!") ||
			strings.HasPrefix(l, "<") || strings.HasPrefix(l, "---") {
			continue
		}
		return l
	}
	return ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func codeList(items []string) []string {
	out := make([]string, len(items))
	for i, s := range items {
		out[i] = "`" + s + "`"
	}
	return out
}
