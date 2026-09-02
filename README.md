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
| [`map`](#ctx3-map) | Symbol index — every type, func, const and var with `file:line` |
| [`functions`](#ctx3-functions) | Function signatures — receivers, args, returns — per file or dir |
| [`flow`](#ctx3-flow) | Go call graph as a text tree or Mermaid flowchart |
| [`impact`](#ctx3-impact) | Reverse call graph — everything that calls a function |
| [`db`](#ctx3-db) | Detected databases + relational schema rebuilt from the repo |
| [`deps`](#ctx3-deps) | Internal package dependency chain + circular‑import detection |
| [`init`](#ctx3-init) | Deterministic `AGENTS.md` / `CLAUDE.md` scaffold for coding agents |
| [`skills`](#emitting-skills) | The whole fact bundle as Claude Code skills (`--list` to audit scopes) |
| [`add-skill`](#installing-your-own-skills) | Install a skill you wrote by hand into a scope |
| [`mcp`](#ctx3-mcp) | Serve the analysis as live MCP tools over stdio |
| [`update`](#updating) | Update ctx3 in place to the latest release |
| `version` | Print the running version |

`context`, `map`, `deps`, `flow` and `db` can each emit their findings as a **[coding‑agent skill](#emitting-skills)** (`--skill`) instead of raw text, so an agent loads the facts only when a task matches them — or serve them **live over [MCP](#ctx3-mcp)**.

### Quick start

```bash
go install github.com/parsabordbar/ctx3@latest

ctx3                         # interactive menu — ↑/↓/Tab to pick, Enter to run
ctx3 print .                 # see the tree
ctx3 pack . -o pack.xml      # pack the repo for an LLM
ctx3 init                    # scaffold an agent context file
ctx3 skills .                # generate the agent skill bundle
```

**[USAGE.md](USAGE.md) is the task-oriented guide** — pick the job (understand a
repo, feed an LLM, set up an agent, guard CI), copy the command. `ctx3 help
<command>` prints the full flag reference for any one command.

Running `ctx3` with no arguments opens an interactive launcher (arrow keys or
Tab to move, Enter to run, `q` to quit). In a pipe or with `--plain` it prints a
static home screen instead.

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
        if cfg.Sections.Structure {
                structure(&buf, tree, cfg)
        }
        if cfg.Sections.Files {
                filesFn(&buf, files, cfg)
        }
        return buf.Bytes(), rep, nil
}
</file>
</files>
Packed 2 files (0 skipped), 2019 bytes
```

**Flags**

* `-o, --output <path>`: write to a file instead of stdout
* `-f, --format xml|md|txt` (default: `xml`) – XML for models, Markdown for humans, plain text for anything that chokes on markup
* `--respect-gitignore` (default: true)
* `--include <glob>[,glob...]`: only include matches (takes precedence over ignores)
* `--ignore <glob>[,glob...]`: exclude matches
* `--max-file-bytes <n>`: skip any single file larger than `n`
* `--max-total-bytes <n>`: stop packing once the total would exceed `n`
* `--budget <tokens>`: the same limit expressed in tokens (`n = tokens × 4`) — see [Token accounting](#token-accounting)
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

### `ctx3 functions`

List every function and method declared in a Go file or directory, with its receiver, arguments and return types. Aliases: `funcs`, `fn`.

Syntax‑only, like [`map`](#ctx3-map) — where `map` indexes all declaration kinds one line each, `functions` is the detailed signature view of one file or package.

**Flags**

* `-r, --recursive`, `-e, --exported`, `--tests`, `-d, --docs`
* `-m, --match <regexp>`, `--sort-name`
* `-g, --grep`, `--md`, `-j, --json`, `-t, --toon`, `-o, --output <path>`

```bash
ctx3 functions pack/walker.go     # one file
ctx3 functions . -r -e            # exported API of the whole repo
ctx3 functions . -r -m '^New' -g  # constructors, grep-style
```

---

### `ctx3 db`

Detect which **databases** a project uses and reconstruct the relational **schema** from files already in the repo — SQL migrations, `schema.sql`, `schema.prisma`, ORM‑tagged Go structs.

Nothing connects to a live database: analysis is static and deterministic. Engines are detected from dependency manifests, container images, and DSNs in env files, and each is reported with the evidence that proved it.

**Flags**

* `-m, --mermaid`: schema as a Mermaid ER diagram
* `--engines-only`: list detected databases, skip the schema
* `-j, --json` / `-t, --toon`: machine‑readable output
* `--skill`, `--as`, `--force`, `-o`: emit a skill — see [Emitting skills](#emitting-skills)

```bash
ctx3 db .
ctx3 db . --mermaid -o schema.md
ctx3 db . --skill                 # schema facts as a Claude Code skill
```

---

### `ctx3 map`

Index **every top‑level symbol** — types, structs, interfaces, classes, functions, methods, consts and vars — each with its signature and `file:line`.

Syntax‑only (no type‑check, no build), so it works on a partial checkout or code that doesn't currently compile. Exported symbols only by default.

**Languages:** Go, TypeScript/JavaScript, Python, Rust, Java and Ruby. Go is read by `go/parser`; the others are matched on declaration patterns, which keeps ctx3 dependency‑free and tolerant of broken files, at the cost of missing an unusually written declaration. Filter with `--lang`.

This is the cheap alternative to reading files: an agent greps the map to find where something lives instead of opening a directory at a time.

**Flags**

* `-a, --all`: include unexported symbols
* `--members`: show struct fields and interface methods
* `-d, --docs`: show the first doc‑comment line under each symbol
* `--kind <list>`: keep only some kinds — `func|method|struct|interface|type|const|var`
* `--lang <list>`: keep only some languages — `go|java|python|ruby|rust|typescript`
* `-m, --match <regexp>`: only symbols whose name matches
* `--tests`: include `_test.go` files
* `--no-recurse`: index only the given directory
* `--budget <tokens>`: drop trailing symbols until the output fits — see [Token accounting](#token-accounting)
* `-g, --grep`: one `file:line: signature` per symbol, for pipes and editors
* `--md`, `-j, --json`, `-t, --toon`: Markdown tables / JSON / TOON
* `-o, --output <path>`: write to a file instead of stdout

**Examples**

```bash
ctx3 map .                          # exported API of the whole repo
ctx3 map . -a                       # include unexported
ctx3 map ./db --members             # struct fields and interface methods
ctx3 map . --kind struct,interface  # the data model only
ctx3 map . --lang python,typescript # one language of a polyglot repo
ctx3 map . -m 'Skill' -g            # grep-style, name filter
```

---

### `ctx3 git`

Report what the repository's **history** says about the code: current branch and HEAD, the uncommitted working set, recent commits, and the files that churn most.

Every other command describes the code as it stands. This one answers "what is actually being worked on here" — the uncommitted list is the working set, the hot‑file list is where effort has been going. Shells out to `git`; no new dependency.

**Flags**

* `--commits <n>`: how many recent commits to list (default 15)
* `--window <n>`: how many commits to measure churn over (default 200)
* `--top <n>`: how many hot files to list (default 15)
* `--md`, `-j, --json`, `-t, --toon`: Markdown tables / JSON / TOON
* `-o, --output <path>`: write to a file instead of stdout
* `--skill`, `--as`, `--force`, `--scope`: emit a skill — see [Emitting skills](#emitting-skills). This one goes stale fastest, so the generated `SKILL.md` says when it was taken and points at `scripts/refresh.sh`.

**Examples**

```bash
ctx3 git .                        # state, log and hot files
ctx3 git . --window 500 --top 25  # churn over more history
ctx3 git . --md -o HISTORY.md
ctx3 git . --skill                # history facts as a Claude Code skill
```

---

### `ctx3 impact`

Walk the call graph **backwards** from a function: direct callers, transitive callers, the packages involved, and the entry points a change can surface at. The blast radius of an edit, before you make it.

The symbol is matched as an exact `pkg.Func` / `pkg.Recv.Method` key first, then by bare function or method name, then as a substring — every match is reported, so an ambiguous name shows all candidates rather than guessing. A function with no callers is reported as such (dead code, or an externally invoked entry point).

Go only. Uses the same graph as [`flow`](#ctx3-flow): type‑checked when the module compiles, parse‑only when it doesn't. On the degraded path a **"no callers" answer is unproven** — the output says so.

**Flags**

* `-C, --dir <path>`: directory to analyze (default `.`)
* `--depth <n>`: caller levels to walk (`0` = unlimited)
* `-m, --mermaid`: reverse call graph as a Mermaid diagram, targets highlighted
* `-j, --json` / `-t, --toon`: machine‑readable output
* `-o, --output <path>`: write to a file instead of stdout

**Examples**

```bash
ctx3 impact Scan                 # who calls any Scan
ctx3 impact symbols.Scan         # one exact function
ctx3 impact "Graph.Skill"        # a method
ctx3 impact Scan --depth 2       # two caller levels
ctx3 impact Scan --mermaid       # diagram of the blast radius
```

---

### `ctx3 brief`

Build the context for **one task** instead of the whole repo. Alias: `task`. The query is a symbol name (`Scan`, `Graph.Skill`) or a free‑text task (`"where do we validate skill names"`); every declaration is ranked against it, and each hit comes back with its **source**, what it **calls**, what **calls** it, the **entry points** it reaches, and its package's imports.

Ranking is deterministic: exact name first, then name / doc / signature / path hits on the query's terms (CamelCase split, stop words dropped). The result is bounded by `--budget`: source snippets shrink first, then the lowest‑ranked hits drop, so the output always fits and the top match is never the part that goes. This is the bridge between `pack` (everything) and `impact` (one symbol).

**Flags**

* `-C, --dir <path>`: directory to analyze (default `.`)
* `--budget <tokens>`: token budget for the output (default 4000, `0` = unlimited)
* `--max <n>`: maximum symbols to include (default 8)
* `--depth <n>`: caller levels to walk (default 3)
* `--snippet <lines>`: maximum source lines per symbol (default 40)
* `-j, --json` / `-t, --toon`: machine‑readable output
* `-o, --output <path>`: write to a file instead of stdout

**Examples**

```bash
ctx3 brief Scan                          # one symbol, everything around it
ctx3 brief "validate skill names"        # a task, best-matching symbols
ctx3 brief Write --budget 1500           # fit a small context window
ctx3 brief Scan -t                       # TOON for an agent
```

---

### `ctx3 diff-context`

Context for a **change**, not a tree. Aliases: `changes`, `diff`. Diffs the working tree against a ref (default `HEAD`, so uncommitted and untracked work), maps every hunk onto the declaration it lands in, walks the call graph backwards from each changed function, and greps the test files that reference it. The output is what a review agent or a CI bot needs: which symbols moved, who depends on them, which entry points they reach, and which tests to run.

**Flags**

* `-C, --dir <path>`: directory to analyze (default `.`)
* `--budget <tokens>`: token budget (`0` = unlimited) — callers trim first, then symbols, then the file list
* `--depth <n>`: caller levels to walk (default 3)
* `-j, --json` / `-t, --toon`: machine‑readable output
* `-o, --output <path>`: write to a file instead of stdout

**Examples**

```bash
ctx3 diff-context                  # uncommitted work vs HEAD
ctx3 diff-context main             # this branch's work vs main
ctx3 diff-context HEAD~3 --budget 2000
ctx3 diff-context origin/main -t   # TOON for a review agent
```

---

### Token accounting

Every command prints a token estimate for what it just wrote on **stderr** (`≈ 1,234 tokens`, bytes ÷ 4, deterministic) so an agent can pick a view by cost. Silence it with the global `--no-tokens`. `pack`, `map`, `flow`, `brief` and `diff-context` take `--budget <tokens>` and trim to fit: `pack` stops adding files, `map` drops trailing symbols (JSON/TOON stay valid), `flow` truncates the rendering on a line boundary, and `brief` / `diff-context` trim structurally so the most important part survives.

---

### `ctx3 flow`

Analyze the **call graph** of a Go project — which function calls which — and render it as a text tree or a Mermaid flowchart.

Calls are resolved with **full type information** (`go/types` via `golang.org/x/tools/go/packages`), so methods called on variables land on their real receiver type (`pkg.Type.Method`), cross‑package calls resolve, and stdlib/vendored noise is filtered out. Execution starts at `func main` **plus every framework command handler** — package‑level vars holding function literals (e.g. cobra `RunE`), which the framework invokes at runtime with no static caller. Each subcommand therefore shows up as its own entry with the domain functions it drives.

**Flags**

* `-m, --mermaid`: output a Mermaid flowchart (pipe to a file with `-o`)
* `-o, --output <path>`: write to a file instead of stdout
* `--entry-only`: keep only functions reachable from an entry point (prunes unreferenced helpers)
* `--depth <n>`: cap traversal depth in the text tree and Mermaid output (`0` = unlimited)
* `--budget <tokens>`: truncate the rendering to fit — see [Token accounting](#token-accounting)
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

Build the **internal package dependency chain**: which packages import which, external deps split out, plus **circular‑import detection**.

Covers **Go, TypeScript/JavaScript and Python** in a single graph — the unit is the directory, so a repo with a Go service and a TS frontend produces one picture rather than two. Go resolves through `go/parser` and the module path; the others resolve from source text, so a `tsconfig` path alias or a specifier built from a variable is dropped rather than guessed. Honors the root `.gitignore`, and no longer requires a `go.mod`.

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

### `ctx3 mcp`

Serve ctx3's analysis over the **Model Context Protocol** so an agent can query it *live, mid‑task*, instead of reading a frozen skill snapshot. Speaks newline‑delimited JSON‑RPC 2.0 over stdin/stdout; it is launched by an MCP client, not run interactively.

**Register with Claude Code**

```bash
claude mcp add ctx3 -- ctx3 mcp
```

or in `.mcp.json`:

```json
{"mcpServers": {"ctx3": {"command": "ctx3", "args": ["mcp"]}}}
```

**Tools exposed** — all read‑only: `ctx3_context`, `ctx3_map`, `ctx3_functions`, `ctx3_deps`, `ctx3_flow`, `ctx3_impact`, `ctx3_db`, `ctx3_tree`, `ctx3_pack`, `ctx3_git`, `ctx3_brief`, `ctx3_diff_context`.

Every result is **bounded**: a compact text rendering by default, truncated past `maxBytes`, so one query against a large repo returns something usable instead of flooding the agent's context.

Skills and MCP are complementary — skills carry the facts that stay true all session, MCP answers the question that comes up mid‑task.

---

### Emitting skills

Some ctx3 commands can package their findings as a **[Claude Code skill](https://docs.claude.com/en/docs/claude-code)** — a `SKILL.md` plus progressive‑disclosure bundle files — instead of dumping text. The point: a coding agent loads a `description`‑matched skill **only when the task needs it**, so heavy per‑domain facts don't bloat the always‑on context file.

`context`, `map`, `deps`, `flow`, `db` and `git` all take `--skill`; `ctx3 skills` emits the whole bundle at once:

```bash
# Write .claude/skills/<name>/SKILL.md + bundle files
ctx3 deps . --skill
ctx3 flow . --skill
ctx3 skills .                     # overview + symbols + deps + flow + db + git in one shot

# Install into your home dir instead of the repo
ctx3 deps . --skill --scope personal

# Overwrite an existing skill dir
ctx3 deps . --skill --force

# Preview the SKILL.md without writing anything
ctx3 deps . --skill -o -
```

**Flags** (on any skill‑capable command)

* `--skill`: emit a skill instead of printing
* `--scope <scope>`: where to install — `enterprise|personal|project` (default `project`), see [Skill scope and priority](#skill-scope-and-priority)
* `--as <target>`: which tool convention to write for (default `claude`) — **only `claude` supports skills**
* `--model <model>`: optional `model:` frontmatter (e.g. `opus`, `sonnet`, `haiku`)
* `--allowed-tools <list>`: optional `allowed-tools:` frontmatter — an allowlist; omit for no restriction
* `--force`: overwrite an existing skill directory
* `-o -`: print the `SKILL.md` to stdout instead of writing files

A generated skill follows the open bundle layout — `references/` for docs, `scripts/` for executables (written with the exec bit, and described in `SKILL.md` as *run, do not read*, so only their output costs tokens), `assets/` for templates and fixtures:

```
.claude/skills/ctx3-deps/
├── SKILL.md                    # name + description (the trigger) + a short overview
├── references/
│   ├── dependencies.md         # facts, loaded on demand
│   └── dependencies.mermaid.md # diagram, loaded on demand
└── scripts/
    └── refresh.sh              # re-derives the facts — run it, never read it
```

Every generated skill ships `scripts/refresh.sh`, which re‑runs the exact command (directory, scope, `--model`, `--allowed-tools`) that produced it. Generated facts go stale as the code changes; the agent fixes that by running one script whose contents never enter the context window — only ctx3's one‑line summary does. Regenerating with `--force` also prunes `references/`, `scripts/` and `assets/` first, so a file from an older generation can't keep serving facts that no longer hold.

`SKILL.md` stays a table of contents, not the document: ctx3 warns when a generated one crosses **500 lines**, which is the signal to push detail down into `references/`.

Under the hood this is the reusable **`skillwriter`** package (`skillwriter.Write` / `Validate` / `Lint`), so new fact‑emitting commands get consistent, validated skill output for free — see [Using ctx3 as a library](#using-ctx3-as-a-library).

---

### Installing your own skills

Write a skill by hand, then install it where an agent will find it:

```bash
ctx3 add-skill ./my-skill --scope personal   # available in every repo
ctx3 add-skill ./my-skill --scope project    # commit it with the code
ctx3 add-skill ./my-skill/SKILL.md           # the SKILL.md path works too
ctx3 add-skill ./review --name backend-review  # install under a different name
ctx3 add-skill ./my-skill --dry-run          # validate + show where it lands
ctx3 skills --list                           # every installed skill, per scope
```

The whole bundle is copied (`SKILL.md`, `references/`, `scripts/`, `assets/`, exec bits preserved) after validating the two things that decide whether the skill is usable at all: a **kebab-case name** and a **non-empty description** — the description is the only text the agent matches a task against. Restart Claude Code after installing, updating (edit `SKILL.md`) or removing (delete the directory) a skill.

### Skill scope and priority

The same skill name can exist in several places. Highest priority wins, and the lower copies never load:

| Priority | Scope | Location | Writable |
|---|---|---|---|
| 1 | `enterprise` | managed settings dir (`/Library/Application Support/ClaudeCode/skills`, `/etc/claude-code/skills`, `%PROGRAMDATA%\ClaudeCode\skills`) | yes (needs admin rights) |
| 2 | `personal` | `~/.claude/skills` | yes |
| 3 | `project` | `<repo>/.claude/skills` | yes |
| 4 | `plugin` | `~/.claude/plugins/**/skills` | no — owned by the plugin |

So an enterprise `code-review` beats your personal `code-review`, which beats the repo's. That's how an organization enforces a standard while individuals still customize. Plugin skills are namespaced `plugin:skill`, so they never collide.

`add-skill` and every `--skill` emitter check the other scopes and warn when the copy they just wrote is shadowed; `ctx3 skills --list` marks shadowed copies with `⊘`. The fix is a descriptive name — `backend-review`, not `review` — either in the frontmatter or via `--name` at install time.

---

## Installation

### Install script (no Go required)

Downloads a prebuilt binary for your OS/arch from the latest GitHub Release:

```bash
curl -fsSL https://raw.githubusercontent.com/parsabordbar/ctx3/main/install.sh | sh
```

Supports macOS and Linux (amd64/arm64). It installs to `/usr/local/bin` when
writable, otherwise `~/.local/bin` — override with `CTX3_INSTALL_DIR`, or pin a
version with `CTX3_VERSION=v0.1.0`. On Windows, download the `.zip` from the
[releases page](https://github.com/parsabordbar/ctx3/releases) or use `go install`.

### With Go

```bash
go install github.com/parsabordbar/ctx3@latest
```

Ensure `$GOPATH/bin` (or your Go install bin dir) is on your `PATH`.

```bash
ctx3 --help
ctx3 version
```

### Shell completion

Tab-complete commands and directory arguments (`ctx3 pa`<kbd>Tab</kbd> → `pack`,
`ctx3 pack `<kbd>Tab</kbd> → directories). Enable it once for your shell:

```bash
# zsh — add to a directory on your $fpath
ctx3 completion zsh > "${fpath[1]}/_ctx3"

# bash
ctx3 completion bash > /usr/local/etc/bash_completion.d/ctx3

# fish
ctx3 completion fish > ~/.config/fish/completions/ctx3.fish
```

Run `ctx3 completion --help` for the full per-shell instructions.

## Updating

Update in place to the latest tagged release:

```bash
ctx3 update            # installs the latest version via the Go toolchain
ctx3 update --check    # just report the latest version, install nothing
```

`update` asks the Go module proxy what `@latest` resolves to, then runs
`go install github.com/parsabordbar/ctx3@latest`. It needs Go on your `PATH`.
Without Go, re-run the install script to grab the latest prebuilt binary:

```bash
curl -fsSL https://raw.githubusercontent.com/parsabordbar/ctx3/main/install.sh | sh
```

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
| `symbols` | `map` | Flat symbol index with `file:line` |
| `funcs` | `functions` | Function signatures, parse‑only |
| `db` | `db` | Detected datastores + reconstructed schema |
| `gitfacts` | `git` | Branch, working set, commits, churn |
| `brief` | `brief` | Task‑scoped context pack under a token budget |
| `diffctx` | `diff-context` | Changed symbols, callers, tests to run |
| `mcp` | `mcp` | Serve the analysis over JSON‑RPC 2.0 on stdio |
| `skillwriter` | `--skill` | Materialize, load and install Claude Code skills |
| `target` | `--as`, `--scope` | Resolve a tool selector / skill scope to its dirs |

```go
import (
    "github.com/parsabordbar/ctx3/filetree"
    "github.com/parsabordbar/ctx3/analyzer"
    "github.com/parsabordbar/ctx3/skillwriter"
)
```

## Roadmap

**Done:** call graph (`flow`, package map `-p`) · reverse call graph (`impact`) · polyglot dependency chain (`deps`) · multi‑language symbol index (`map`) · database + schema analysis (`db`) · repository history (`git`) · task briefs (`brief`) · change context (`diff-context`) · token budgets (`--budget`) · agent context files (`init`) · skill emission and installation (`skills`, `add-skill`, `skillwriter`) · MCP server (`mcp`) · `pack` in XML / Markdown / plain text

**Next:**

- YAML output
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
