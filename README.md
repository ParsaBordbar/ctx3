# Context Tree (ctx3)

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![Build](https://img.shields.io/github/actions/workflow/status/parsabordbar/ctx3/ci.yml?label=build)](https://github.com/parsabordbar/ctx3/actions)
[![Downloads](https://img.shields.io/github/downloads/parsabordbar/ctx3/total.svg)](https://github.com/parsabordbar/ctx3/releases)
[![License](https://img.shields.io/github/license/parsabordbar/ctx3)](LICENSE)
[![Stars](https://img.shields.io/github/stars/parsabordbar/ctx3?style=social)](https://github.com/parsabordbar/ctx3)

<p align="center">
  <img width="200" alt="ctx3" src="https://github.com/user-attachments/assets/7cca9bd3-5587-4df0-a7c1-c5b4323d6a8e" />
</p>


**Context Tree (ctx3)** is a free, open-source CLI tool written in Go that helps you (and your favorite LLM) understand a codebase better by providing structured metadata about files and dependencies.

---

## What Can It Do?

ctx3 turns a repo into structured, LLM‑friendly facts — from a quick file tree to a packed artifact you can hand to a model, a Go call‑graph, a dependency chain, and a ready‑to‑commit agent context file.

### At a glance

| Command | What you get |
|---|---|
| [`print`](#ctx3-print) | File hierarchy of the project |
| [`context`](#ctx3-context) | File/dep metadata + README preview (text, JSON, or TOON) |
| [`percentage`](#ctx3-percentage) | Language / file‑type breakdown |
| [`pack`](#ctx3-pack) | Whole repo packed into one AI‑friendly file (Repomix‑style) |
| [`flow`](#ctx3-flow) | Go call graph as a text tree or Mermaid flowchart |
| [`deps`](#ctx3-deps) | Internal package dependency chain + circular‑import detection |
| [`init`](#ctx3-init) | Deterministic `AGENTS.md` / `CLAUDE.md` scaffold for coding agents |
| [`update`](#updating) | Update ctx3 in place to the latest release |
| `version` | Print the running version |

Two of these — `deps` and (soon) DB analysis — can also emit their findings as a **[coding‑agent skill](#emitting-skills)** instead of raw text, so an agent loads the facts only when they're relevant.

### Quick start

```bash
go install github.com/parsabordbar/ctx3@latest

ctx3 print .                 # see the tree
ctx3 pack . -o pack.xml      # pack the repo for an LLM
ctx3 init                    # scaffold an agent context file
```

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

### `ctx3 flow`

Analyze the **call graph** of a Go project — which function calls which — and render it as a text tree or a Mermaid flowchart.

Calls are resolved with **full type information** (`go/types` via `golang.org/x/tools/go/packages`), so methods called on variables land on their real receiver type (`pkg.Type.Method`), cross‑package calls resolve, and stdlib/vendored noise is filtered out. Execution starts at `func main` **plus every framework command handler** — package‑level vars holding function literals (e.g. cobra `RunE`), which the framework invokes at runtime with no static caller. Each subcommand therefore shows up as its own entry with the domain functions it drives.

**Flags**

* `-m, --mermaid`: output a Mermaid flowchart (pipe to a file with `-o`)
* `-o, --output <path>`: write to a file instead of stdout
* `--entry-only`: keep only functions reachable from an entry point (prunes unreferenced helpers)
* `--depth <n>`: cap traversal depth in the text tree and Mermaid output (`0` = unlimited)
* `--skill`, `--as`, `--force`: emit the call graph as a skill — see [Emitting skills](#emitting-skills)

**Examples**

```bash
ctx3 flow .
ctx3 flow . --entry-only --depth 3   # per-command app flow, 3 levels deep
ctx3 flow . --mermaid -o flow.md
ctx3 flow . --skill                # call-graph facts as a Claude Code skill
```

---

### `ctx3 deps`

Build the **internal package dependency chain** of a Go module: which packages import which, external deps split out, plus **circular‑import detection**.

**Flags**

* `-m, --mermaid`: dependency graph as Mermaid markdown
* `-j, --json` / `-t, --toon`: machine‑readable output (TOON is compact, LLM‑optimized)
* `-o, --output <path>`: write to a file instead of stdout
* `--cycles-only`: report only circular imports (non‑zero exit if any found — handy in CI)
* `--quiet`: with `--cycles-only`, print only on failure (exit code is the signal)
* `--skill`, `--as`, `--force`: emit a skill instead of printing — see [Emitting skills](#emitting-skills)

**Examples**

```bash
ctx3 deps .
ctx3 deps . --mermaid -o deps.md
ctx3 deps . --cycles-only         # fails the build on a cycle
ctx3 deps . --cycles-only --quiet # CI: silent unless a cycle exists
ctx3 deps . -t                 # compact TOON for an LLM
```

---

### `ctx3 init`

Generate a **context file for coding agents** — an `AGENTS.md` (default) or `CLAUDE.md` — by composing ctx3's own analysis: project metadata, detected build/test/run commands, package + call‑graph architecture, a depth‑limited **directory tree**, language breakdown, dependencies, and a **key‑files** list. Output is **deterministic (no LLM)**; `TODO` markers flag what a human should refine.

The `Structure` tree and `Key files` sections are noise‑filtered (lockfiles, `node_modules`, `target/`, generated artifacts dropped) and entry points are surfaced first — so a large Rust/Python repo shows its real layout, not eight lockfiles.

**Flags**

* `--as agent|claude|gemini|copilot`: which tool's file to write (`agent` → `AGENTS.md`, `claude` → `CLAUDE.md`, `gemini` → `GEMINI.md`, `copilot` → `.github/copilot-instructions.md`)
* `-o, --output <path>`: override the output path
* `--stdout`: print instead of writing a file
* `--force`: overwrite an existing file

**Examples**

```bash
ctx3 init                      # writes AGENTS.md
ctx3 init . --as claude        # writes CLAUDE.md
ctx3 init --stdout             # preview without writing
```

---

### Emitting skills

Some ctx3 commands can package their findings as a **[Claude Code skill](https://docs.claude.com/en/docs/claude-code)** — a `SKILL.md` plus progressive‑disclosure `reference/` files — instead of dumping text. The point: a coding agent loads a `description`‑matched skill **only when the task needs it**, so heavy per‑domain facts don't bloat the always‑on context file.

Both `deps --skill` (dependency chain) and `flow --skill` (call graph) emit skills:

```bash
# Write .claude/skills/<name>/SKILL.md + reference files
ctx3 deps . --skill
ctx3 flow . --skill

# Overwrite an existing skill dir
ctx3 deps . --skill --force

# Preview the SKILL.md without writing anything
ctx3 deps . --skill -o -
```

**Flags** (on any skill‑capable command)

* `--skill`: emit a skill instead of printing
* `--as <target>`: which tool convention to write for (default `claude`) — **only `claude` supports skills**
* `--force`: overwrite an existing skill directory
* `-o -`: print the `SKILL.md` to stdout instead of writing files

A generated skill looks like:

```
.claude/skills/ctx3-deps/
├── SKILL.md                    # name + description (the trigger) + a short overview
└── reference/
    ├── dependencies.md         # facts, loaded on demand
    └── dependencies.mermaid.md # diagram, loaded on demand
```

Under the hood this is the reusable **`skillwriter`** package (`skillwriter.Write` / `Validate`), so new fact‑emitting commands get consistent, validated skill output for free — see [Using ctx3 as a library](#using-ctx3-as-a-library).

---

## Installation

Make sure you have Go installed. Then:

```bash
go install github.com/parsabordbar/ctx3@latest
```

Ensure `$GOPATH/bin` (or your Go install bin dir) is on your `PATH`.

```bash
ctx3 --help
ctx3 version
```

## Updating

Update in place to the latest tagged release:

```bash
ctx3 update            # installs the latest version via the Go toolchain
ctx3 update --check    # just report the latest version, install nothing
```

`update` asks the Go module proxy what `@latest` resolves to, then runs
`go install github.com/parsabordbar/ctx3@latest`. It needs Go on your `PATH`
(same as the install step). Without Go, run that `go install` line manually.

## Build From Source

```bash
git clone https://github.com/parsabordbar/ctx3.git
cd ctx3
go build -o ctx3
# optionally: mv ctx3 /usr/local/bin/
```

## Using ctx3 as a Library

Besides being a CLI tool, ctx3 can be imported directly into your Go projects — every command is a thin adapter over a reusable package:

| Package | Backs | Use it for |
|---|---|---|
| `filetree` | `print` | Walk + print a file hierarchy |
| `analyzer` | `context` | Project metadata + dep list from `go.mod` |
| `pack` | `pack` | Pack a repo into one artifact |
| `flow` | `flow` | Build a Go call graph |
| `deps` | `deps` | Internal dependency chain + cycle detection |
| `agentmd` | `init` | Compose an agent context file |
| `skillwriter` | `--skill` | Materialize a Claude Code skill from derived facts |
| `target` | `--as` | Resolve a tool selector to its files/skill dir |

```go
import (
    "github.com/parsabordbar/ctx3/filetree"
    "github.com/parsabordbar/ctx3/analyzer"
    "github.com/parsabordbar/ctx3/skillwriter"
)
```

## Roadmap

**Done:** call graph (`flow`) · dependency chain (`deps`) · agent context files (`init`) · skill emission (`skillwriter`)

**Next:**

- Markdown / TXT renderers for `pack`
- YAML output
- Database type + relation analysis (emitted as a skill)
- Datagrams / ER diagrams
- Gist — code‑snippet extraction
- Prompt generation

## Contributing

Contributions welcome! Open issues or PRs. If you're proposing a larger change, please start a discussion first.

**Workflow:** `main` is always releasable and protected — branch off it (`feat/…`, `fix/…`), open a PR, get CI green (`go test ./...` + `go build`), then squash‑merge. Use [Conventional Commit](https://www.conventionalcommits.org) titles.

## Releasing (maintainers)

ctx3 has no publish step — `go install` pulls straight from Git via the module proxy, and **"latest" means the highest semver tag**. To cut a release:

```bash
git checkout main && git pull
git tag v0.1.0            # semver: vMAJOR.MINOR.PATCH
git push origin v0.1.0
```

Within minutes `go install github.com/parsabordbar/ctx3@latest` (and `ctx3 update`) resolve to the new tag. `ctx3 version` reports it automatically — the version is read from the module build info, no `-ldflags` needed.

Rules of thumb: bump PATCH for fixes, MINOR for features, MAJOR for breaking changes. Reaching `v1.0.0` promises API stability (a `v2` would require a `/v2` module‑path suffix — avoid until necessary). Prebuilt binaries via GoReleaser can be added later so non‑Go users can install without the toolchain.
