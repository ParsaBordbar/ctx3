package filetree

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

//go:embed ignore.json
var ignoreFile []byte

type DirIgnores struct {
	Dirs []string `json:"dirs"`
}

func readIgnoreJson() []string {
	var fields DirIgnores
	err := json.Unmarshal(ignoreFile, &fields)
	if err != nil {
		panic(err)
	}
	return fields.Dirs
}

func PrintTree(root string, prefix string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for i, entry := range entries {
		if slices.Contains(readIgnoreJson(), entry.Name()) {
			continue
		}

		path := filepath.Join(root, entry.Name())
		info, _ := os.Stat(path)

		isLast := i == len(entries)-1

		branch := "├── "
		newPrefix := prefix + "│   "
		if isLast {
			branch = "└── "
			newPrefix = prefix + "    "
		}

		fmt.Println(prefix + branch + entry.Name() + fmt.Sprintf(" (%d bytes)", info.Size()))

		if entry.IsDir() {
			PrintTree(path, newPrefix)
		}
	}
}