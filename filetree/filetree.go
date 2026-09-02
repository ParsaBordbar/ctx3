package filetree

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed ignore.json
var ignoreFile []byte

type DirIgnores struct {
	Dirs []string `json:"dirs"`
}

// ignored is the embedded skip list, decoded once. It used to be unmarshalled
// inside the walk loop, which re-parsed the JSON for every entry in the tree.
var ignored = sync.OnceValue(func() map[string]bool {
	var fields DirIgnores
	if err := json.Unmarshal(ignoreFile, &fields); err != nil {
		// The file is embedded at build time, so this cannot fail in a built
		// binary; refusing to skip anything beats taking the process down.
		return map[string]bool{}
	}
	set := make(map[string]bool, len(fields.Dirs))
	for _, d := range fields.Dirs {
		set[d] = true
	}
	return set
})

// Config controls a tree rendering.
type Config struct {
	// Root is the directory to walk.
	Root string
	// MaxDepth caps how deep the tree goes (0 = unlimited). A deep tree is the
	// main way this output runs away with a caller's context.
	MaxDepth int
	// Sizes appends "(N bytes)" to each entry.
	Sizes bool
}

// Render returns the directory tree as a string.
//
// It returns rather than prints so the tree can be written to a file, embedded
// in a larger document, or served over MCP — a handler that printed to stdout
// would corrupt the JSON-RPC stream it shares.
func Render(cfg Config) string {
	root := cfg.Root
	if root == "" {
		root = "."
	}
	var sb strings.Builder
	writeTree(&sb, root, "", cfg, 0)
	return sb.String()
}

// PrintTree writes the tree of root to stdout, prefixing every line.
func PrintTree(root string, prefix string) {
	var sb strings.Builder
	writeTree(&sb, root, prefix, Config{Root: root, Sizes: true}, 0)
	fmt.Print(sb.String())
}

func writeTree(sb *strings.Builder, dir, prefix string, cfg Config, depth int) {
	if cfg.MaxDepth > 0 && depth >= cfg.MaxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(sb, "%s[unreadable: %v]\n", prefix, err)
		return
	}

	skip := ignored()
	kept := entries[:0]
	for _, e := range entries {
		if !skip[e.Name()] {
			kept = append(kept, e)
		}
	}

	// The branch glyph depends on being the last *kept* entry, so filtering has
	// to happen before the loop — filtering inside it drew a "└──" mid-tree
	// whenever the real last entry was an ignored one.
	for i, entry := range kept {
		isLast := i == len(kept)-1
		branch, childPrefix := "├── ", prefix+"│   "
		if isLast {
			branch, childPrefix = "└── ", prefix+"    "
		}

		sb.WriteString(prefix + branch + entry.Name())
		if cfg.Sizes {
			if info, err := entry.Info(); err == nil && !entry.IsDir() {
				fmt.Fprintf(sb, " (%d bytes)", info.Size())
			}
		}
		sb.WriteByte('\n')

		if entry.IsDir() {
			writeTree(sb, filepath.Join(dir, entry.Name()), childPrefix, cfg, depth+1)
		}
	}
}
