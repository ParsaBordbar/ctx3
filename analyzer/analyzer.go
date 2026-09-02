package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type FileInfo struct {
	Name         string `json:"name" toon:"name"`
	Type         string `json:"type" toon:"type"`
	Path         string `json:"path" toon:"path"`
	Size         int64  `json:"size" toon:"size"`
	Lines        int    `json:"lines" toon:"lines"`
	LastEdited   string `json:"lastEdited" toon:"last_edited"`
	IsEntryPoint bool   `json:"isEntryPoint" toon:"is_entry_point"`
}

// Language is one file type's share of the project, measured in bytes.
type Language struct {
	Ext     string  `json:"ext" toon:"ext"`
	Files   int     `json:"files" toon:"files"`
	Bytes   int64   `json:"bytes" toon:"bytes"`
	Percent float64 `json:"percent" toon:"percent"`
}

type ProjectContext struct {
	Root       string     `json:"root" toon:"root"`
	Files      []FileInfo `json:"files" toon:"files"`
	TotalFiles int        `json:"total_files" toon:"total_files"`
	TotalDirs  int        `json:"total_dirs" toon:"total_dirs"`
	Languages  []Language `json:"languages" toon:"languages"`
	// Dependencies are the module's *direct* requirements only. The indirect
	// block of a go.mod is transitive noise — dozens of entries that say
	// nothing about what the project chose to depend on — so it is reported as
	// a count rather than a list.
	Dependencies  []string `json:"dependencies" toon:"dependencies"`
	IndirectCount int      `json:"indirect_count" toon:"indirect_count"`
	Readme        string   `json:"readme" toon:"readme"`
}

// entryFileNames are conventional program entry points, matched by file name.
var entryFileNames = map[string]bool{
	"main.go": true, "server.go": true,
	"index.js": true, "app.js": true, "server.js": true,
	"main.ts": true, "app.ts": true, "index.ts": true, "server.ts": true,
	"main.py": true, "app.py": true, "index.py": true,
	"main.rb": true, "app.rb": true, "index.rb": true, "server.rb": true,
	"main.php": true, "app.php": true, "index.php": true, "server.php": true,
}

// skipDirs are never descended into, at any depth. Matching on the base name
// (not the path relative to the root) is what makes a nested `web/node_modules`
// or `services/api/vendor` skip too — otherwise their contents dominate the
// file counts and language percentages of any polyglot repo.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, ".venv": true, "venv": true,
	"__pycache__": true, ".next": true, ".idea": true, ".vscode": true,
}

var OutputJSON bool
var OutputTOON bool

// maxLineCountBytes caps the per-file line scan. Above it the file is a blob
// (a lockfile, a bundle, an asset) whose line count is not worth the read.
const maxLineCountBytes = 2 << 20

// readmePreviewLimit is how much README prose to keep, in runes.
const readmePreviewLimit = 400

// readFileString reads a file, returning "" if it cannot be read.
func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// parseGoModRequires extracts deps from a go.mod, handling both the single-line
// `require path v1.2.3` and block `require ( ... )` forms. It returns the direct
// requirements and the number of `// indirect` ones, which are dropped.
func parseGoModRequires(data string) (direct []string, indirect int) {
	inBlock := false
	add := func(line string) {
		dep, isIndirect := cleanRequire(line)
		switch {
		case dep == "":
		case isIndirect:
			indirect++
		default:
			direct = append(direct, dep)
		}
	}
	for raw := range strings.SplitSeq(data, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case inBlock:
			if line == ")" {
				inBlock = false
				continue
			}
			add(line)
		case line == "require (":
			inBlock = true
		case strings.HasPrefix(line, "require "):
			add(strings.TrimSpace(line[len("require "):]))
		}
	}
	return direct, indirect
}

// cleanRequire strips a trailing comment and reports whether it marked the
// requirement indirect.
func cleanRequire(line string) (dep string, indirect bool) {
	if i := strings.Index(line, "//"); i >= 0 {
		indirect = strings.Contains(line[i:], "indirect")
		line = line[:i]
	}
	return strings.TrimSpace(line), indirect
}

func countLines(path string, size int64) int {
	if size > maxLineCountBytes {
		return 0
	}
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	return lines
}

func isEntryPoint(name string) bool {
	return entryFileNames[name]
}

// readmePreview pulls the first real prose out of a README. Taking a raw byte
// prefix is near-useless: on a typical project the first 300 bytes are the
// title and a wall of shields.io badges, which tells a reader nothing about
// what the project does (and can cut mid-rune, producing invalid UTF-8).
func readmePreview(data []byte, limit int) string {
	var title, body []string
	used := 0
	for raw := range strings.SplitSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case isBadgeLine(line):
			continue
		case strings.HasPrefix(line, "<"): // raw HTML banners/centering
			continue
		case isRuleLine(line): // --- / *** / ___ separators
			continue
		case strings.HasPrefix(line, "#"):
			// The heading marker is kept: consumers distinguish the title from
			// the blurb by it.
			if len(title) == 0 && len(body) == 0 {
				title = append(title, line)
			}
			continue
		}
		body = append(body, line)
		if used += len(line); used >= limit {
			break
		}
	}

	out := strings.TrimSpace(strings.Join(append(title, body...), "\n"))
	if len([]rune(out)) > limit {
		out = string([]rune(out)[:limit]) + "…"
	}
	return out
}

// isRuleLine reports whether a line is a markdown horizontal rule.
func isRuleLine(line string) bool {
	return strings.Trim(line, "-") == "" || strings.Trim(line, "*") == "" ||
		strings.Trim(line, "_") == "" || strings.Trim(line, "=") == ""
}

// badgeRE matches a markdown image, optionally wrapped in a link — the shape
// every shields.io badge takes, either `![alt](src)` or `[![alt](src)](href)`.
var badgeRE = regexp.MustCompile(`\[?!\[[^\]]*\]\([^)]*\)(\]\([^)]*\))?`)

// isBadgeLine reports whether a line is nothing but badges: once the badge
// markup is removed, no prose is left.
func isBadgeLine(line string) bool {
	if !strings.Contains(line, "![") {
		return false
	}
	return strings.TrimSpace(badgeRE.ReplaceAllString(line, "")) == ""
}

func CollectFileStats(ctx *ProjectContext) map[string]int64 {
	counts := make(map[string]int64)

	for _, file := range ctx.Files {
		ext := file.Type
		if ext == "" {
			ext = "other"
		}
		counts[ext] += file.Size
	}
	return counts
}

func FilePercentage(counts map[string]int64) map[string]float64 {
	var total int64
	for _, v := range counts {
		total += v
	}

	percentages := make(map[string]float64)
	for ext, v := range counts {
		percentages[ext] = (float64(v) / float64(total)) * 100
	}
	return percentages
}

// RenderPercentage returns the file-type breakdown as a string. Callers that
// are not a terminal — a file, a pipe, an MCP handler whose stdout carries a
// JSON-RPC stream — need the text back rather than printed, and pass color
// false to leave the ANSI escapes out.
func RenderPercentage(percentages map[string]float64, color bool) string {
	// Map iteration order is random, so sort: biggest share first, name as the
	// tiebreak, otherwise the same project prints in a different order each run.
	keys := make([]string, 0, len(percentages))
	for k := range percentages {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if percentages[keys[i]] != percentages[keys[j]] {
			return percentages[keys[i]] > percentages[keys[j]]
		}
		return keys[i] < keys[j]
	})

	var sb strings.Builder
	sb.WriteString("┌── File Percentages:\n")
	for i, lang := range keys {
		percent := percentages[lang]

		branch := "├── "
		if i == len(keys)-1 {
			branch = "└── "
		}

		bar := strings.Repeat("█", int(percent/2))
		if color {
			esc := "\033[36m"
			if i%2 == 0 {
				esc = "\033[34m"
			}
			bar = esc + bar + "\033[0m"
		}

		fmt.Fprintf(&sb, "%s%-10s %5.1f%% %s\n", branch, lang, percent, bar)
	}
	return sb.String()
}

// PrettyPrintPercentage writes the breakdown to stdout, in color.
func PrettyPrintPercentage(percentages map[string]float64) {
	fmt.Print(RenderPercentage(percentages, true))
}

func AnalyzeProject(root string) ProjectContext {
	ctx := ProjectContext{Root: root}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		if info.IsDir() {
			// The root itself is never skipped, even if it happens to be named
			// like one of the excluded directories.
			if rel != "." && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			ctx.TotalDirs++
			return nil
		}

		ext := strings.TrimPrefix(filepath.Ext(info.Name()), ".")
		fileType := "file"
		if ext != "" {
			fileType = ext
		}

		ctx.Files = append(ctx.Files, FileInfo{
			Name:         info.Name(),
			IsEntryPoint: isEntryPoint(info.Name()),
			Type:         fileType,
			Path:         rel,
			Size:         info.Size(),
			Lines:        countLines(path, info.Size()),
			LastEdited:   info.ModTime().UTC().Format(time.RFC3339),
		})
		ctx.TotalFiles++

		if strings.ToLower(info.Name()) == "readme.md" && ctx.Readme == "" {
			data, _ := os.ReadFile(path)
			ctx.Readme = readmePreview(data, readmePreviewLimit)
		}

		if info.Name() == "go.mod" {
			direct, indirect := parseGoModRequires(readFileString(path))
			ctx.Dependencies = append(ctx.Dependencies, direct...)
			ctx.IndirectCount += indirect
		}
		return nil
	})

	ctx.Languages = CollectLanguages(&ctx)
	return ctx
}

// CollectLanguages summarizes the file mix by type, largest share first. Shares
// are by total bytes, so one big generated file does not read as many small
// hand-written ones.
func CollectLanguages(ctx *ProjectContext) []Language {
	bytes := CollectFileStats(ctx)
	if len(bytes) == 0 {
		return nil
	}
	counts := make(map[string]int, len(bytes))
	for _, f := range ctx.Files {
		ext := f.Type
		if ext == "" {
			ext = "other"
		}
		counts[ext]++
	}

	pct := FilePercentage(bytes)
	langs := make([]Language, 0, len(bytes))
	for ext, n := range bytes {
		langs = append(langs, Language{Ext: ext, Files: counts[ext], Bytes: n, Percent: pct[ext]})
	}
	sortByShare(langs)
	return langs
}

// sortByShare orders languages by share, then by name so equal shares do not
// reorder between runs.
func sortByShare(langs []Language) {
	sort.Slice(langs, func(i, j int) bool {
		if langs[i].Percent != langs[j].Percent {
			return langs[i].Percent > langs[j].Percent
		}
		return langs[i].Ext < langs[j].Ext
	})
}
