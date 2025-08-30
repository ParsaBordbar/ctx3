package analyzer

import (
	"os"
	"path/filepath"
	"strings"
)

type FileInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type ProjectContext struct {
	Root        string     `json:"root"`
	Files       []FileInfo `json:"files"`
	TotalFiles  int        `json:"total_files"`
	TotalDirs   int        `json:"total_dirs"`
	Dependencies []string  `json:"dependencies,omitempty"`
	Readme      string     `json:"readme,omitempty"`
}

var OutputJSON bool

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
				Type: fileType,
				Path: rel,
				Size: info.Size(),
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