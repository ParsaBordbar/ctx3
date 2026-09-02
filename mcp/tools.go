package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/parsabordbar/ctx3/analyzer"
	"github.com/parsabordbar/ctx3/brief"
	"github.com/parsabordbar/ctx3/db"
	"github.com/parsabordbar/ctx3/deps"
	"github.com/parsabordbar/ctx3/diffctx"
	"github.com/parsabordbar/ctx3/filetree"
	"github.com/parsabordbar/ctx3/flow"
	"github.com/parsabordbar/ctx3/funcs"
	"github.com/parsabordbar/ctx3/gitfacts"
	"github.com/parsabordbar/ctx3/pack"
	"github.com/parsabordbar/ctx3/symbols"
)

// Tool is one MCP tool: a name, a trigger description, a JSON-Schema for its
// arguments, and a handler that returns a string (or an error, surfaced to the
// agent as an isError tool result).
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema map[string]any
	Handler     func(args map[string]any) (string, error)
}

// defaultMaxBytes bounds a single tool result. Analysis payloads grow with the
// repo — a whole call graph as JSON is tens of thousands of tokens even for a
// small module — and an unbounded result would swamp the caller's context,
// which is the opposite of what querying facts live is for. Over the cap the
// result is truncated with a marker telling the agent how to narrow the query.
const defaultMaxBytes = 60000

// defaultTools is the read-only tool set: one thin wrapper per ctx3 analysis
// package. Handlers add no analysis logic — they parse args, call the package,
// and render the result live against the current tree.
func defaultTools() []Tool {
	return []Tool{
		{
			Name:  "ctx3_context",
			Title: "Project overview",
			Description: "Project overview: file/directory counts, dependency list, language mix and README " +
				"preview. Call to orient on an unfamiliar repo before deeper queries.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":   dirProp,
				"files": prop("boolean", "Include the full file list (json format only; large on big repos)."),
			}), nil),
			Handler: handleContext,
		},
		{
			Name:  "ctx3_map",
			Title: "Symbol index",
			Description: "Symbol index: every top-level type, struct, interface, class, function, method, " +
				"const and var with its signature and file:line. Call to find where a symbol is defined or " +
				"what a module exports without reading files. Covers Go, TypeScript/JavaScript, Python, " +
				"Rust, Java and Ruby. Parse-only — works on non-compiling code.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":     dirProp,
				"kind":    prop("string", "Comma-separated kinds to keep (func,method,struct,interface,type,const,var)."),
				"lang":    prop("string", "Comma-separated languages to keep (go,typescript,python,rust,java,ruby)."),
				"match":   prop("string", "Only symbols whose name matches this regexp."),
				"all":     prop("boolean", "Include unexported symbols (default: exported only)."),
				"members": prop("boolean", "Include struct fields and interface methods."),
			}), nil),
			Handler: handleMap,
		},
		{
			Name:  "ctx3_functions",
			Title: "Function signatures",
			Description: "Detailed function and method signatures: receiver, generic type parameters, each " +
				"parameter and result, plus the doc line and file:line. Call when you need a function's exact " +
				"shape rather than just where it lives (use ctx3_map for the broader symbol index). Parse-only.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":       prop("string", "A .go file or directory to scan (default: current directory)."),
				"recursive": prop("boolean", "Walk subdirectories (default: the given directory only)."),
				"match":     prop("string", "Only functions whose name matches this regexp."),
				"all":       prop("boolean", "Include unexported functions (default: exported only)."),
				"tests":     prop("boolean", "Include _test.go files."),
			}), nil),
			Handler: handleFunctions,
		},
		{
			Name:  "ctx3_deps",
			Title: "Package dependency graph",
			Description: "Internal package dependency graph: which directories import which, external " +
				"dependencies, and any circular imports. Call to reason about coupling or safe " +
				"refactor/extraction order. Covers Go, TypeScript/JavaScript and Python in one graph, " +
				"so a repo with a Go service and a TS frontend gives a single picture.",
			InputSchema: schema(withCommon(map[string]any{"dir": dirProp}), nil),
			Handler:     handleDeps,
		},
		{
			Name:  "ctx3_flow",
			Title: "Call graph",
			Description: "Call graph: which functions call which, plus the entry points where execution starts. " +
				`Start with view:"packages" to see how the system fans out in one screen, then view:"functions" ` +
				"to trace a specific path. Go only. Type-checks the module when it compiles and falls back to " +
				"parse-only analysis when it does not, flagging the result as degraded. Use depth or " +
				"entryOnly to keep the function view small on a large repo.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":       dirProp,
				"view":      prop("string", `"packages" (default) for the package-level fan-out map, "functions" for the full call tree.`),
				"entryOnly": prop("boolean", "Keep only functions reachable from entry points (functions view)."),
				"depth":     prop("integer", "Cap traversal depth, 0 = unlimited (functions view)."),
			}), nil),
			Handler: handleFlow,
		},
		{
			Name:  "ctx3_impact",
			Title: "Change blast radius",
			Description: "Blast radius: everything that transitively calls a symbol — direct and transitive " +
				"callers, the packages involved, and reachable entry points. Call before changing a function " +
				"to see what it can break. Go only. Works on a tree that does not compile, but marks the " +
				"result degraded — treat a \"no callers\" answer as unproven when it is.",
			InputSchema: schema(withCommon(map[string]any{
				"symbol": prop("string", "Symbol to analyze: pkg.Func, pkg.Recv.Method, a bare name, or a substring."),
				"dir":    dirProp,
				"depth":  prop("integer", "Caller levels to walk (0 = unlimited)."),
			}), []string{"symbol"}),
			Handler: handleImpact,
		},
		{
			Name:  "ctx3_db",
			Title: "Database schema",
			Description: "Databases the project uses and any relational schema that can be reconstructed from " +
				"migrations, models or DDL: tables, columns, keys and relationships. Call before writing a " +
				"query or a migration, or to learn the data model.",
			InputSchema: schema(withCommon(map[string]any{"dir": dirProp}), nil),
			Handler:     handleDB,
		},
		{
			Name:  "ctx3_tree",
			Title: "Directory tree",
			Description: "Directory tree of the project, with build output, dependencies and virtualenvs " +
				"filtered out. Call to see how a repo is laid out before deciding what to read. Use depth " +
				"to keep it small — the full tree of a large repo is thousands of lines.",
			InputSchema: schema(map[string]any{
				"dir":      dirProp,
				"depth":    prop("integer", "Levels to show, 0 = unlimited. Start with 2 or 3."),
				"sizes":    prop("boolean", "Append each file's size in bytes."),
				"maxBytes": maxBytesProp,
			}, nil),
			Handler: handleTree,
		},
		{
			Name:  "ctx3_pack",
			Title: "Packed repository",
			Description: "The repository packed into one document: a directory tree plus file contents, " +
				"the artifact you hand a model when it needs to read real code rather than an index. " +
				`Defaults to the tree alone — pass section:"all" with include globs to pull actual contents, ` +
				"since packing a whole repo will not fit in a reply. Prefer ctx3_map to locate code and this " +
				"to read a specific subtree.",
			InputSchema: schema(map[string]any{
				"dir":     dirProp,
				"section": prop("string", `"structure" (default, the tree only), "files" (contents only), or "all".`),
				"style":   prop("string", `Document format: "md" (default), "xml" or "txt".`),
				"include": prop("string", `Comma-separated globs to include, e.g. "auth/**,*.go". Strongly recommended with section:"all".`),
				"ignore":  prop("string", "Comma-separated globs to exclude."),
				"maxFileBytes": prop("integer",
					"Skip any single file larger than this (default 65536)."),
				"maxBytes": maxBytesProp,
			}, nil),
			Handler: handlePack,
		},
		{
			Name:  "ctx3_git",
			Title: "Repository history",
			Description: "What the repository's history says about the code: current branch and HEAD, the " +
				"uncommitted working set, recent commits, and the files that change most often. Call to find " +
				"out what is actively being worked on — the one thing reading the source cannot tell you. " +
				"Requires a git checkout.",
			InputSchema: schema(map[string]any{
				"dir":      dirProp,
				"commits":  prop("integer", "How many recent commits to list (default 15)."),
				"window":   prop("integer", "How many commits to measure file churn over (default 200)."),
				"top":      prop("integer", "How many hot files to list (default 15)."),
				"maxBytes": maxBytesProp,
			}, nil),
			Handler: handleGit,
		},
		{
			Name:  "ctx3_brief",
			Title: "Task brief",
			Description: "Context for one task instead of the whole repo: rank every declaration against a " +
				"symbol name or a free-text task, and for each hit return its source, what it calls, what " +
				"calls it, the entry points it reaches and its package's imports — trimmed to a token budget. " +
				"Call this first when starting a change; it replaces reading a directory at a time.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":    dirProp,
				"query":  prop("string", `A symbol ("Scan", "Graph.Skill") or a task ("where do we validate skill names").`),
				"budget": prop("integer", "Token budget for the result (default 4000, 0 = unlimited)."),
				"max":    prop("integer", "Maximum symbols to include (default 8)."),
				"depth":  prop("integer", "Caller levels to walk (default 3)."),
			}), []string{"query"}),
			Handler: handleBrief,
		},
		{
			Name:  "ctx3_diff_context",
			Title: "Change context",
			Description: "What a change touches: the working tree diffed against a ref (default HEAD, so " +
				"uncommitted and untracked work), each hunk mapped to the declaration it lands in, the callers " +
				"and entry points of every changed function, and the test files that reference it. Call before " +
				"reviewing or committing to know what to re-test.",
			InputSchema: schema(withCommon(map[string]any{
				"dir":    dirProp,
				"ref":    prop("string", `Git ref to diff against (default "HEAD"; e.g. "main", "HEAD~3").`),
				"budget": prop("integer", "Token budget for the result (0 = unlimited)."),
				"depth":  prop("integer", "Caller levels to walk (default 3)."),
			}), nil),
			Handler: handleDiffContext,
		},
	}
}

func handleBrief(args map[string]any) (string, error) {
	q := strings.TrimSpace(argStr(args, "query", ""))
	if q == "" {
		return "", fmt.Errorf("query is required")
	}
	b, err := brief.Build(brief.Config{
		RootDir:    argStr(args, "dir", "."),
		Query:      q,
		Budget:     argInt(args, "budget", 4000),
		MaxSymbols: argInt(args, "max", 0),
		Depth:      argInt(args, "depth", 0),
	})
	if err != nil {
		return "", err
	}
	return result(args, func() string { return brief.RenderText(b) }, b)
}

func handleDiffContext(args map[string]any) (string, error) {
	c, err := diffctx.Build(diffctx.Config{
		RootDir: argStr(args, "dir", "."),
		Ref:     argStr(args, "ref", ""),
		Budget:  argInt(args, "budget", 0),
		Depth:   argInt(args, "depth", 0),
	})
	if err != nil {
		return "", err
	}
	return result(args, func() string { return diffctx.RenderText(c) }, c)
}

// --- handlers ---

func handleContext(args map[string]any) (string, error) {
	dir := argStr(args, "dir", ".")
	ctx := analyzer.AnalyzeProject(dir)

	// The full file list dominates the JSON payload and is rarely what the
	// caller wants, so it is opt-in.
	jsonVal := any(ctx)
	if !argBool(args, "files") {
		trimmed := ctx
		trimmed.Files = nil
		jsonVal = trimmed
	}
	return result(args, func() string { return renderContextText(&ctx) }, jsonVal)
}

func handleMap(args map[string]any) (string, error) {
	cfg := symbols.Config{
		Path:              argStr(args, "dir", "."),
		IncludeUnexported: argBool(args, "all"),
		Members:           argBool(args, "members"),
	}
	if kinds := strings.TrimSpace(argStr(args, "kind", "")); kinds != "" {
		for raw := range strings.SplitSeq(kinds, ",") {
			if k := strings.ToLower(strings.TrimSpace(raw)); k != "" {
				cfg.Kinds = append(cfg.Kinds, symbols.Kind(k))
			}
		}
	}
	if langs := strings.TrimSpace(argStr(args, "lang", "")); langs != "" {
		for raw := range strings.SplitSeq(langs, ",") {
			if l := strings.ToLower(strings.TrimSpace(raw)); l != "" {
				cfg.Langs = append(cfg.Langs, l)
			}
		}
	}
	re, err := argRegexp(args, "match")
	if err != nil {
		return "", err
	}
	cfg.Match = re

	idx, err := symbols.Scan(cfg)
	if err != nil {
		return "", err
	}
	return result(args, func() string { return symbols.RenderGrep(idx) }, idx)
}

func handleFunctions(args map[string]any) (string, error) {
	cfg := funcs.Config{
		Path:         argStr(args, "dir", "."),
		Recursive:    argBool(args, "recursive"),
		ExportedOnly: !argBool(args, "all"),
		IncludeTests: argBool(args, "tests"),
	}
	re, err := argRegexp(args, "match")
	if err != nil {
		return "", err
	}
	cfg.Match = re

	res, err := funcs.Scan(cfg)
	if err != nil {
		return "", err
	}
	return result(args, func() string { return funcs.RenderText(res, true) }, res)
}

func handleDeps(args map[string]any) (string, error) {
	g, err := deps.Analyze(deps.Config{RootDir: argStr(args, "dir", ".")})
	if err != nil {
		return "", err
	}
	return result(args, func() string { return deps.RenderText(g) }, g)
}

func handleFlow(args map[string]any) (string, error) {
	dir := argStr(args, "dir", ".")
	g, err := flow.AnalyzeFlow(flow.Config{
		RootDir:   dir,
		EntryOnly: argBool(args, "entryOnly"),
		MaxDepth:  argInt(args, "depth", 0),
	})
	if err != nil {
		return "", err
	}

	// The package view is the default: the function-level graph runs to tens of
	// thousands of tokens on a real repo, and "how does this fan out" is the
	// question a caller usually has before it has a specific function in mind.
	switch view := strings.ToLower(argStr(args, "view", "packages")); view {
	case "packages", "":
		pf := flow.PackageGraph(g, projectName(dir))
		return result(args, func() string { return flow.RenderPackageFlow(pf) }, pf)
	case "functions":
		return result(args, func() string { return flow.RenderText(g) }, g)
	default:
		return "", fmt.Errorf("unknown view %q (want: packages, functions)", view)
	}
}

// projectName labels a rendering with the module base, falling back to the
// directory name.
func projectName(dir string) string {
	if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if mod, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
				mod = strings.TrimSpace(mod)
				if i := strings.LastIndex(mod, "/"); i >= 0 {
					return mod[i+1:]
				}
				return mod
			}
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return filepath.Base(abs)
	}
	return "project"
}

func handleImpact(args map[string]any) (string, error) {
	sym := strings.TrimSpace(argStr(args, "symbol", ""))
	if sym == "" {
		return "", fmt.Errorf("symbol is required")
	}
	g, err := flow.AnalyzeFlow(flow.Config{RootDir: argStr(args, "dir", ".")})
	if err != nil {
		return "", err
	}
	imp, err := flow.Impacted(g, sym, argInt(args, "depth", 0))
	if err != nil {
		return "", err
	}
	return result(args, func() string { return flow.RenderImpactText(imp) }, imp)
}

func handleDB(args map[string]any) (string, error) {
	rep, err := db.Analyze(db.Config{RootDir: argStr(args, "dir", ".")})
	if err != nil {
		return "", err
	}
	return result(args, func() string { return db.RenderText(rep) }, rep)
}

func handleTree(args map[string]any) (string, error) {
	out := filetree.Render(filetree.Config{
		Root:     argStr(args, "dir", "."),
		MaxDepth: argInt(args, "depth", 0),
		Sizes:    argBool(args, "sizes"),
	})
	return truncate(out, argInt(args, "maxBytes", defaultMaxBytes)), nil
}

// defaultPackFileBytes keeps one oversized file — a lockfile, a fixture, a
// generated blob — from consuming the whole budget before the code is reached.
const defaultPackFileBytes = 64 << 10

func handlePack(args map[string]any) (string, error) {
	cfg := pack.Config{
		RootDir:          argStr(args, "dir", "."),
		RespectGitignore: true,
		IncludeGlobs:     argList(args, "include"),
		IgnoreGlobs:      argList(args, "ignore"),
		MaxFileBytes:     int64(argInt(args, "maxFileBytes", defaultPackFileBytes)),
		Compact:          true,
	}

	switch style := strings.ToLower(argStr(args, "style", "md")); style {
	case "md", "":
		cfg.OutputFormat = pack.FormatMD
	case "xml":
		cfg.OutputFormat = pack.FormatXML
	case "txt":
		cfg.OutputFormat = pack.FormatTXT
	default:
		return "", fmt.Errorf("unknown style %q (want: md, xml, txt)", style)
	}

	// The tree alone is the safe default: a caller that asks for "the repo"
	// without narrowing it should get something that fits, not a truncated
	// dump that spent its whole budget on the first directory alphabetically.
	switch section := strings.ToLower(argStr(args, "section", "structure")); section {
	case "structure", "":
		cfg.Sections = pack.Sections{Structure: true}
	case "files":
		cfg.Sections = pack.Sections{Files: true}
	case "all":
		cfg.Sections = pack.Sections{Structure: true, Files: true}
	default:
		return "", fmt.Errorf("unknown section %q (want: structure, files, all)", section)
	}

	out, rep, err := pack.Pack(context.Background(), cfg)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Packed %d file(s), %d skipped, %d bytes.\n\n", rep.FilesIncluded, rep.FilesSkipped, rep.TotalBytes)
	b.Write(out)
	return truncate(b.String(), argInt(args, "maxBytes", defaultMaxBytes)), nil
}

// argList splits a comma-separated argument into trimmed, non-empty entries.
func argList(args map[string]any, key string) []string {
	raw := strings.TrimSpace(argStr(args, key, ""))
	if raw == "" {
		return nil
	}
	var out []string
	for piece := range strings.SplitSeq(raw, ",") {
		if p := strings.TrimSpace(piece); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func handleGit(args map[string]any) (string, error) {
	rep, err := gitfacts.Analyze(gitfacts.Config{
		RootDir:     argStr(args, "dir", "."),
		Commits:     argInt(args, "commits", 0),
		ChurnWindow: argInt(args, "window", 0),
		TopFiles:    argInt(args, "top", 0),
	})
	if err != nil {
		return "", err
	}
	return truncate(gitfacts.RenderText(rep), argInt(args, "maxBytes", defaultMaxBytes)), nil
}

// renderContextText is the compact overview: counts, language mix, direct
// dependencies and the README preview, without the per-file listing.
func renderContextText(ctx *analyzer.ProjectContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s\n", ctx.Root)
	fmt.Fprintf(&b, "Files: %d, Dirs: %d\n", ctx.TotalFiles, ctx.TotalDirs)

	if langs := languageMix(ctx); langs != "" {
		fmt.Fprintf(&b, "\nLanguages:\n%s", langs)
	}
	if len(ctx.Dependencies) > 0 {
		fmt.Fprintf(&b, "\nDependencies (%d direct", len(ctx.Dependencies))
		if ctx.IndirectCount > 0 {
			fmt.Fprintf(&b, ", %d indirect not listed", ctx.IndirectCount)
		}
		b.WriteString("):\n")
		for _, d := range ctx.Dependencies {
			fmt.Fprintf(&b, "  %s\n", d)
		}
	}
	if ctx.Readme != "" {
		fmt.Fprintf(&b, "\nREADME preview:\n%s\n", ctx.Readme)
	}
	return b.String()
}

// languageMix renders the file-type breakdown, largest share first.
func languageMix(ctx *analyzer.ProjectContext) string {
	var b strings.Builder
	for i, l := range ctx.Languages {
		if i == 8 { // a long tail of one-off extensions is noise
			fmt.Fprintf(&b, "  (+%d more)\n", len(ctx.Languages)-i)
			break
		}
		noun := "files"
		if l.Files == 1 {
			noun = "file"
		}
		fmt.Fprintf(&b, "  %-8s %5.1f%%  (%d %s)\n", l.Ext, l.Percent, l.Files, noun)
	}
	return b.String()
}

// --- argument + schema helpers ---

// dirProp is the directory argument shared by every tool.
var dirProp = prop("string", "Directory to analyze (default: current directory).")

// maxBytesProp is the result cap, for tools that render text only and so do not
// take the common format/maxBytes pair.
var maxBytesProp = prop("integer", fmt.Sprintf("Truncate the result past this many bytes (default %d).", defaultMaxBytes))

// withCommon adds the arguments every tool accepts to a tool's own properties.
func withCommon(props map[string]any) map[string]any {
	props["format"] = prop("string", `"text" (default) for the compact rendering, "json" for the full structured payload. Prefer text: it costs a fraction of the tokens.`)
	props["maxBytes"] = prop("integer", fmt.Sprintf("Truncate the result past this many bytes (default %d).", defaultMaxBytes))
	return props
}

// result renders a tool's answer in the caller's requested format and bounds it.
// The text rendering is built lazily so the JSON path never pays for it.
func result(args map[string]any, text func() string, jsonVal any) (string, error) {
	var s string
	if strings.ToLower(argStr(args, "format", "text")) == "json" {
		b, err := json.MarshalIndent(jsonVal, "", "  ")
		if err != nil {
			return "", err
		}
		s = string(b)
	} else {
		s = text()
	}
	return truncate(s, argInt(args, "maxBytes", defaultMaxBytes)), nil
}

// truncate caps s at max bytes, cutting on a line boundary and appending a
// marker so the agent can tell the answer is partial and narrow its query
// rather than treating what it got as the whole picture.
func truncate(s string, max int) string {
	if max <= 0 {
		max = defaultMaxBytes
	}
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return fmt.Sprintf("%s\n\n[truncated: %d of %d bytes shown. Narrow the query (dir, match, kind, depth, entryOnly) or raise maxBytes.]\n",
		cut, len(cut), len(s))
}

func argStr(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

func argBool(args map[string]any, key string) bool {
	v, _ := args[key].(bool)
	return v
}

// argInt tolerates JSON numbers, which decode as float64.
func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// argRegexp compiles an optional regexp argument, reporting a bad pattern as a
// tool error rather than silently ignoring the filter.
func argRegexp(args map[string]any, key string) (*regexp.Regexp, error) {
	pattern := strings.TrimSpace(argStr(args, key, ""))
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid %s pattern: %w", key, err)
	}
	return re, nil
}

// prop builds one JSON-Schema property node. The property's name is the key it
// is stored under in the enclosing properties map, not part of the node.
func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// schema builds a JSON-Schema object from properties and required keys.
func schema(props map[string]any, required []string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
