<p align="center">
  <img width="200" alt="ctx3" src="https://github.com/user-attachments/assets/7cca9bd3-5587-4df0-a7c1-c5b4323d6a8e" />
</p>

# Context Tree (ctx3)

**Context Tree (ctx3)** is a free, open-source CLI tool written in Go that helps you (and your favorite LLM) understand a codebase better by providing structured metadata about files and dependencies.

---

## What Can It Do?

ctx3 combines three core ideas:

1. **File Tree** – print the file hierarchy of your project.
2. **Context** – collect metadata about files, dependencies, and a README preview.
3. **Pack** – generate a single, LLM‑friendly file that contains the directory structure and the full contents of files (similar to Repomix). Supports ignore/include globs, binary handling, size caps, and a compact mode.

---

## Commands

### `ctx3 context`

Outputs metadata (optionally as JSON) including file sizes, types, dependencies, and README contents.

<img width="3796" height="996" alt="context" src="https://github.com/user-attachments/assets/b81f102a-8cf6-467c-9f69-29f677396c9d" />

**Examples**

```bash
# Human‑readable
ctx3 context

# JSON output
ctx3 context -j
```

**Sample JSON**

```json
{
  "root": ".",
  "files": [
    {
      "name": "ctx3",
      "type": "file",
      "path": "ctx3",
      "size": 3832706,
      "lines": 4901,
      "lastEdited": "2025-09-06 01:39:56.680487278 +0330 +0330"
    },
    {
      "name": "filetree.go",
      "type": "go",
      "path": "filetree/filetree.go",
      "size": 565,
      "lines": 25,
      "lastEdited": "2025-09-04 20:15:01.949704998 +0330 +0330"
    },
    {
      "name": "main.go",
      "type": "go",
      "path": "main.go",
      "size": 88,
      "lines": 7,
      "lastEdited": "2025-08-31 00:05:09.234305186 +0330 +0330",
      "isEntryPoint": true
    }
  ],
  "total_files": 11,
  "total_dirs": 4,
  "dependencies": ["github.com/spf13/cobra v1.9.1"]
}
```

---

### `ctx3 print`

Prints the file hierarchy of your project and shows the structure.

<img width="1396" height="1380" alt="code" src="https://github.com/user-attachments/assets/771f5e41-42db-4977-85c1-e54be0abf139" />

**Example**

```bash
ctx3 print .
```

---

### `ctx3 percentage`

See which languages / file types dominate a codebase.

<img width="1396" height="932" alt="code" src="https://github.com/user-attachments/assets/ab7d9b2e-ec07-4f25-a358-39b0c2764fda" />

**Example**

```bash
ctx3 percentage
```

---

### `ctx3 pack`

Pack a repository into a single AI‑friendly artifact (XML‑ish), containing a `<directory_structure>` section and a `<files>` section with each file’s contents.

**Why?** Handy for LLM workflows where you need to paste or upload a whole repo at once (similar to Repomix).

**Default output shape**

```xml
<directory_structure>
config.go
pack.go
</directory_structure>
<files>
This section contains the contents of the repository's files.
<file path="config.go">
package pack

import "runtime"

type OutputFormat string

const (
        FormatXML OutputFormat = "xml"
        FormatMD  OutputFormat = "md"
        FormatTXT OutputFormat = "txt"
)

type BinaryStrategy string

const (
        BinarySkip   BinaryStrategy = "skip"
        BinaryHex    BinaryStrategy = "hex"
        BinaryBase64 BinaryStrategy = "base64"
)

type Sections struct {
        Structure bool
        Files     bool
}

type Config struct {
        RootDir          string
        OutputFormat     OutputFormat
        OutputPath       string
        RespectGitignore bool
        IncludeGlobs     []string
        IgnoreGlobs      []string
        MaxFileBytes     int64
        MaxTotalBytes    int64
        BinaryHandling   BinaryStrategy
        SortByExt        bool // false = sort by path
        Sections         Sections
        RedactPatterns   []string
        Concurrency      int // 0 or <0 => auto

        // When true, removes extra blank lines between sections and files.
        Compact bool
}

type FileEntry struct {
        RelPath  string
        Size     int64
        IsBinary bool
        Content  []byte // omitted when skipped
}

type Report struct {
        FilesIncluded int
        FilesSkipped  int
        TotalBytes    int64
        Warnings      []string
}

func (c *Config) normalizedConcurrency() int {
        if c.Concurrency <= 0 {
                return runtime.NumCPU()
        }
        return c.Concurrency
}
</file>
<file path="pack.go">
package pack

import (
        "bytes"
        "context"
        "fmt"
)

// Pack walks the repository and renders the output into a single buffer.
// Currently supports XML (sample-style). MD/TXT can be added later.
func Pack(ctx context.Context, cfg Config) ([]byte, Report, error) {
        files, tree, rep, err := WalkAndCollect(ctx, cfg)
        if err != nil {
                return nil, rep, err
        }

        var buf bytes.Buffer
        switch cfg.OutputFormat {
        case FormatXML:
                if cfg.Sections.Structure {
                        renderXMLStructure(&buf, tree, cfg)
                }
                if cfg.Sections.Files {
                        renderXMLFiles(&buf, files, cfg)
                }
        default:
                return nil, rep, fmt.Errorf("unsupported format: %s (only xml is implemented)", cfg.OutputFormat)
        }

        return buf.Bytes(), rep, nil
}
</file>
</files>
Packed 2 files (0 skipped), 2019 bytes
```

**Flags**

* `-o, --output <path>`: write to a file instead of stdout
* `-f, --format xml|md|txt` (default: `xml`) – *currently XML implemented*
* `--respect-gitignore` (default: true)
* `--include <glob>[,glob...]`: only include matches (takes precedence over ignores)
* `--ignore <glob>[,glob...]`: exclude matches
* `--max-file-bytes <n>`: skip any single file larger than `n`
* `--max-total-bytes <n>`: stop packing once the total would exceed `n`
* `--binary skip|hex|base64` (default: `skip`): how to include binary files
* `--sort paths|ext` (default: `paths`): deterministic ordering
* `--section all|structure|files` (default: `all`) – choose which sections to output
* `--redact <regex>[,regex...]`: redact content by regex (replaced with `***`)
* `--concurrency <n>`: number of concurrent file reads (default: auto)
* `--compact`: remove extra blank lines between blocks

**Examples**

```bash
# Basic pack to stdout
ctx3 pack .

# Respect .gitignore, skip binaries, write to file
ctx3 pack . --binary skip -o pack.xml

# Include only .go and README, ignore vendor folder, compact spacing
ctx3 pack . --include "**/*.go,README.md" --ignore "vendor/**" --compact -o pack.xml

# Enforce size limits
ctx3 pack . --max-file-bytes 200000 --max-total-bytes 5000000 -o pack.xml
```

> **Notes**
>
> * Globs use `**` for recursive matches. Patterns like `**/*.go` match in all subfolders. If you want basename-only patterns, prefer explicit `**/`.
> * `.git` and `node_modules` are always excluded from traversal.
> * `.gitignore` at repo root is respected by default.

---

## Installation

Make sure you have Go installed. Then:

```bash
go install github.com/parsabordbar/ctx3@latest
```

Ensure `$GOPATH/bin` (or your Go install bin dir) is on your `PATH`.

```bash
ctx3 --help
```

## Build From Source

```bash
git clone https://github.com/parsabordbar/ctx3.git
cd ctx3
go build -o ctx3
# optionally: mv ctx3 /usr/local/bin/
```

## Using ctx3 as a Library

Besides being a CLI tool, ctx3 can also be imported directly into your Go projects.  
All the `filetree`, `analyzer` and `pack` packages are designed to be reusable.  
You can pull them in with a standard Go import:

```go
import (
    "github.com/parsabordbar/ctx3/filetree"
    "github.com/parsabordbar/ctx3/analyzer"
)
```

## Roadmap

- Markdown/TXT renderers for `pack`
- Support for Prompt Generations
- Code-Base Tech Detection (Similar to github)
- Gist (Code snipt extraction support)

## Controbutions 
Contributions welcome! Feel free to open issues or PRs. If you’re proposing a larger change, please start a discussion first.