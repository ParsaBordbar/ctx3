package filetree

import (
	"fmt"
	"os"
	"path/filepath"
)

func PrintTree(root string, indent string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		fmt.Println("Error reading directory:", err)
		return
	}

	for _, entry := range entries {
		if entry.Name() != "node_modules" && entry.Name() != ".git" {
			info, _ := os.Stat(filepath.Join(root, entry.Name()))
			fmt.Println(indent + "├── " + entry.Name() + fmt.Sprintf(" (%d bytes)", info.Size()))
			if entry.IsDir() {
				PrintTree(filepath.Join(root, entry.Name()), indent+"    ")
			}
		}
	}
}
