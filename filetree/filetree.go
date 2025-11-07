package filetree

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"encoding/json"
)

type DirIgnores struct {
	Dirs []string `json:"dirs"`
}

func readIgnoreJson() []string {
	data, err := os.ReadFile("ignoretree.json")
	if err != nil {
		panic(err)
	}

	var fileds DirIgnores
	err = json.Unmarshal(data, &fileds)
	if err != nil {
		panic(err)
	}
	return fileds.Dirs
}

func PrintTree(root string, prefix string) {

	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for i, entry := range entries {
		if slices.Contains(readIgnoreJson() ,entry.Name()) {
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
