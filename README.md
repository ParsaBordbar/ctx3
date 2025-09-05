
<p align="center">
  <img width="200" alt="ctx3" src="https://github.com/user-attachments/assets/7cca9bd3-5587-4df0-a7c1-c5b4323d6a8e" />
</p>

# Context Tree (ctx3)

**Context Tree (ctx3)** is a free, open-source CLI tool written in Go that helps you (and your favorite LLM) understand a codebase better by providing structured metadata about files and dependencies.


##  What Can It Do?

ctx3 combines two core ideas:

1. **File Tree** – a CLI tool that shows the file hierarchy of your project.
2. **Context** – provides metadata and summaries about files and dependencies.

Together, these help LLMs (and developers) reason about your codebase more effectively.

---

###  File Tree
<img width="463" height="409" alt="Screenshot" src="https://github.com/user-attachments/assets/3d2ab86c-4ee3-4e80-a7ef-35e9b1ddacbf" />

Prints the file hierarchy of your project and shows the structure.

### Context
Outputs metadata (optionally as JSON) including file sizes, types, dependencies, and README contents.

Example:

```bash
ctx3 context -j
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
### Installation

First, make sure you have Go installed. If not, follow the [Go installation guide](https://go.dev/doc/install).

Then install ctx3 using:

```bash
go install github.com/parsabordbar/ctx3@latest
```

Make sure ``$GOPATH/bin (or Go install dir) `` is in your ``PATH``.
Now you can run:

```bash
ctx3 --help
```
OR
```bash
ctx3 help
```

### Build From Source

If you want to compile manually:

#### 1- Clone The Repo:
```
git clone https://github.com/parsabordbar/ctx3.git
```
#### 2- Go To Repo:
```
cd ctx3
```
#### 3- Build:
```
go build -o ctx3
```
#### 4- Move To Path
This will create a ctx3 binary in the current directory. Move it somewhere in your PATH, for example:
```
mv ctx3 /usr/local/bin/
```

## Usage

Print a file tree (defaults to current directory):
```
ctx3 print <path>
```


Get metadata context (defaults to current directory):
```
ctx3 context <path>
```
Options:

--json, -j → output JSON

--help, -h → show help for a command

## Using ctx3 as a Library

Besides being a CLI tool, ctx3 can also be imported directly into your Go projects.  
Both the `filetree` and `analyzer` packages are designed to be reusable.  
You can pull them in with a standard Go import:

```go
import (
    "github.com/parsabordbar/ctx3/filetree"
    "github.com/parsabordbar/ctx3/analyzer"
)
```

## Future Updates

- Support for Prompt Generations
- Code-Base Tech Detection (Similar to github)
- Gist (Code snipt extraction support)

## Controbutions 
You can send Pull Requests and contact me
