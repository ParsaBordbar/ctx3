package filetree

import (
	"fmt"
	"os"
	"path/filepath"
)

func PrintTree(root string, prefix string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for i, entry := range entries {
		if entry.Name() == "node_modules" || entry.Name() == ".git" {
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
