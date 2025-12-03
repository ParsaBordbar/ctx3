package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

type ProjectContext struct {
	Root         string     `json:"root" toon:"root"`
	Files        []FileInfo `json:"files" toon:"files"`
	TotalFiles   int        `json:"total_files" toon:"total_files"`
	TotalDirs    int        `json:"total_dirs" toon:"total_dirs"`
	Dependencies []string   `json:"dependencies" toon:"dependencies"`
	Readme       string     `json:"readme" toon:"readme"`
}

var entryFileNames [20]string = [20]string{
	"main.go", "index.js", "app.py", "server.js", "main.py",
	"app.js", "index.py", "server.go", "main.ts", "app.ts",
	"index.ts", "server.ts", "main.rb", "app.rb", "index.rb",
	"server.rb", "main.php", "app.php", "index.php", "server.php",
}

var OutputJSON bool
var OutputTOON bool

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func countLines(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
	}
	return lines
}

func is_entry_point(name string) bool {
	for _, entry := range entryFileNames {
		if name == entry {
			return true
		}
	}
	return false
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

func PrettyPrintPercentage(percentages map[string]float64) {
	keys := make([]string, 0, len(percentages))
	println("┌── File Percentages:")
	for k := range percentages {
		keys = append(keys, k)
	}

	for i, lang := range keys {
		percent := percentages[lang]
		isLast := i == len(keys)-1

		branch := "├── "
		if isLast {
			branch = "└── "
		}
		color := "\033[36m"
		if i%2 == 0 {
			color = "\033[34m"
		}

		bar := strings.Repeat("█", int(percent/2))
		coloredBar := color + bar + "\033[0m"

		fmt.Printf("%s%-10s %5.1f%% %s\n", branch, lang, percent, coloredBar)
	}
}

func AnalyzeProject(root string) ProjectContext {
	ctx := ProjectContext{Root: root}

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)

		if rel == "node_modules" || rel == ".git" {
			return filepath.SkipDir
		}

		if !info.IsDir() {
			ext := strings.TrimPrefix(filepath.Ext(info.Name()), ".")
			CollectFileStats(&ctx)
			fileType := "file"
			if ext != "" {
				fileType = ext
			}

			ctx.Files = append(ctx.Files, FileInfo{
				Name:         info.Name(),
				IsEntryPoint: is_entry_point(info.Name()),
				Type:         fileType,
				Path:         rel,
				Size:         info.Size(),
				Lines:        countLines(path),
				LastEdited:   info.ModTime().String(),
			})
			ctx.TotalFiles++

			if strings.ToLower(info.Name()) == "readme.md" {
				data, _ := os.ReadFile(path)
				ctx.Readme = string(data[:min(300, len(data))]) + "..."
			}

			if info.Name() == "go.mod" {
				data, _ := os.ReadFile(path)
				lines := strings.Split(string(data), "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "require ") {
						ctx.Dependencies = append(ctx.Dependencies, strings.TrimSpace(line[8:]))
					}
				}
			}
		} else {
			ctx.TotalDirs++
		}
		return nil
	})

	return ctx
}