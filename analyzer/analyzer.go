package analyzer

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type FileInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Lines int   `json:"lines,omitempty"`
	LastEdited string `json:"lastEdited,omitempty"`
	IsEntryPoint bool   `json:"isEntryPoint,omitempty"`
}

type ProjectContext struct {
	Root        string     `json:"root"`
	Files       []FileInfo `json:"files"`
	TotalFiles  int        `json:"total_files"`
	TotalDirs   int        `json:"total_dirs"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Readme      string     `json:"readme,omitempty"`
}

var entryFileNames [20] string = [20]string{
	"main.go", "index.js", "app.py", "server.js", "main.py",
	"app.js", "index.py", "server.go", "main.ts", "app.ts",
	"index.ts", "server.ts", "main.rb", "app.rb", "index.rb",
	"server.rb", "main.php", "app.php", "index.php", "server.php",
}

var OutputJSON bool

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
			fileType := "file"
			if ext != "" {
				fileType = ext
			}

			ctx.Files = append(ctx.Files, FileInfo{
				Name: info.Name(),
				IsEntryPoint: is_entry_point(info.Name()),
				Type: fileType,
				Path: rel,
				Size: info.Size(),
				Lines: countLines(path),
				LastEdited: info.ModTime().String(),
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