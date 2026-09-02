package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffold writes a throwaway Go module with the given files (relpath -> content)
// and returns its root dir.
func scaffold(t *testing.T, module string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	files["go.mod"] = "module " + module + "\n\ngo 1.22\n"
	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// writeFile writes one file, creating parent directories.
func writeFile(t *testing.T, full, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyze_InternalVsExternal(t *testing.T) {
	root := scaffold(t, "example.com/app", map[string]string{
		"main.go":    "package main\nimport (\n\t\"fmt\"\n\t\"example.com/app/store\"\n)\nfunc main() { fmt.Println(store.X) }\n",
		"store/s.go": "package store\nimport \"strings\"\nvar X = strings.ToUpper(\"x\")\n",
	})

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if g.Module != "example.com/app" {
		t.Fatalf("module = %q", g.Module)
	}
	if len(g.List) != 2 {
		t.Fatalf("want 2 packages, got %d", len(g.List))
	}

	main := g.Packages["example.com/app"]
	if len(main.Imports) != 1 || main.Imports[0] != "example.com/app/store" {
		t.Errorf("main internal imports = %v", main.Imports)
	}
	// fmt is external (stdlib), store is internal.
	if len(main.External) != 1 || main.External[0] != "fmt" {
		t.Errorf("main external imports = %v", main.External)
	}
}

func TestAnalyze_DetectsCycle(t *testing.T) {
	root := scaffold(t, "example.com/cyc", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/cyc/b\"\n",
		"b/b.go": "package b\nimport _ \"example.com/cyc/a\"\n",
	})

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Cycles) != 1 {
		t.Fatalf("want 1 cycle, got %d: %v", len(g.Cycles), g.Cycles)
	}
	if got := cycleKey(g.Cycles[0]); got != "example.com/cyc/a->example.com/cyc/b" {
		t.Errorf("cycle key = %q", got)
	}
}

func TestAnalyze_NoCycleWhenAcyclic(t *testing.T) {
	root := scaffold(t, "example.com/dag", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/dag/b\"\n",
		"b/b.go": "package b\n",
	})
	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Cycles) != 0 {
		t.Fatalf("expected acyclic, got cycles %v", g.Cycles)
	}
}

func TestAnalyze_TestFilesIgnored(t *testing.T) {
	root := scaffold(t, "example.com/tf", map[string]string{
		"a/a.go":      "package a\n",
		"a/a_test.go": "package a\nimport _ \"example.com/tf/b\"\n",
		"b/b.go":      "package b\n",
	})
	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	// b is only imported from a test file, so a must have no internal imports.
	if a := g.Packages["example.com/tf/a"]; len(a.Imports) != 0 {
		t.Errorf("test-file import leaked into graph: %v", a.Imports)
	}
}

// A project with no go.mod is no longer an error: the graph is directory-keyed
// and the root name falls back to package.json, then to the directory name.
func TestAnalyze_NoGoModUsesPackageJSONName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"webapp"}`)
	writeFile(t, filepath.Join(root, "index.js"), "console.log(1)\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatalf("a non-Go project should analyze: %v", err)
	}
	if g.Module != "webapp" {
		t.Errorf("module = %q, want webapp", g.Module)
	}
}

func TestAnalyze_NoManifestFallsBackToDirName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.py"), "x = 1\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatalf("a bare directory should analyze: %v", err)
	}
	if g.Module != filepath.Base(root) {
		t.Errorf("module = %q, want the directory name %q", g.Module, filepath.Base(root))
	}
}

func TestAnalyze_TypeScriptRelativeImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app"}`)
	writeFile(t, filepath.Join(root, "src/index.ts"),
		"import {Client} from './api/client';\nimport React from 'react';\n")
	writeFile(t, filepath.Join(root, "src/api/client.ts"),
		"import {log} from '../util/log';\nexport class Client {}\n")
	writeFile(t, filepath.Join(root, "src/util/log.ts"), "export function log() {}\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}

	src := g.Packages["app/src"]
	if src == nil {
		t.Fatalf("src package missing, have %v", g.Order)
	}
	if len(src.Imports) != 1 || src.Imports[0] != "app/src/api" {
		t.Errorf("src imports = %v, want [app/src/api]", src.Imports)
	}
	// A bare specifier is a node_modules dependency, not an internal edge.
	if len(src.External) != 1 || src.External[0] != "react" {
		t.Errorf("src external = %v, want [react]", src.External)
	}
	if api := g.Packages["app/src/api"]; api == nil || len(api.Imports) != 1 || api.Imports[0] != "app/src/util" {
		t.Errorf("api imports = %v, want [app/src/util]", api.Imports)
	}
}

func TestAnalyze_TypeScriptSameDirIsNotAnEdge(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"app"}`)
	writeFile(t, filepath.Join(root, "src/a.ts"), "import {b} from './b';\n")
	writeFile(t, filepath.Join(root, "src/b.ts"), "export const b = 1;\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if src := g.Packages["app/src"]; src == nil || len(src.Imports) != 0 {
		t.Errorf("a sibling file is not a package edge: %v", src)
	}
}

func TestAnalyze_PythonImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app/main.py"),
		"import os\nfrom app.store import Repo\nfrom .util import helper\n")
	writeFile(t, filepath.Join(root, "app/store.py"), "class Repo: pass\n")
	writeFile(t, filepath.Join(root, "app/util.py"), "def helper(): pass\n")
	writeFile(t, filepath.Join(root, "app/db/conn.py"), "from ..store import Repo\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(root)

	app := g.Packages[name+"/app"]
	if app == nil {
		t.Fatalf("app package missing, have %v", g.Order)
	}
	// os is stdlib; app.store and .util resolve inside app, which is the same
	// directory, so neither is a package edge.
	if len(app.Imports) != 0 {
		t.Errorf("app imports = %v, want none (all same-directory)", app.Imports)
	}
	if !contains(app.External, "os") {
		t.Errorf("stdlib import missing from external: %v", app.External)
	}
	// A relative import climbing out of db/ is a real edge.
	if conn := g.Packages[name+"/app/db"]; conn == nil || len(conn.Imports) != 1 || conn.Imports[0] != name+"/app" {
		t.Errorf("app/db imports = %v, want [%s/app]", conn, name+"/app")
	}
}

func TestAnalyze_MixedGoAndTypeScriptShareOneGraph(t *testing.T) {
	root := scaffold(t, "example.com/mix", map[string]string{
		"api/api.go": "package api\nimport _ \"example.com/mix/store\"\n",
		"store/s.go": "package store\n",
	})
	writeFile(t, filepath.Join(root, "web/index.ts"), "import {c} from './lib/client';\n")
	writeFile(t, filepath.Join(root, "web/lib/client.ts"), "export const c = 1;\n")

	g, err := Analyze(Config{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if g.Packages["example.com/mix/api"] == nil {
		t.Error("Go package missing from mixed graph")
	}
	if web := g.Packages["example.com/mix/web"]; web == nil || len(web.Imports) != 1 {
		t.Errorf("TypeScript package missing or unlinked: %v", web)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestRenderText_And_Mermaid(t *testing.T) {
	root := scaffold(t, "example.com/cyc", map[string]string{
		"a/a.go": "package a\nimport _ \"example.com/cyc/b\"\n",
		"b/b.go": "package b\nimport _ \"example.com/cyc/a\"\n",
	})
	g, _ := Analyze(Config{RootDir: root})

	txt := RenderText(g)
	if !strings.Contains(txt, "Circular imports detected") {
		t.Errorf("text render missing cycle warning:\n%s", txt)
	}
	mer := RenderMermaid(g)
	if !strings.Contains(mer, "flowchart LR") || !strings.Contains(mer, "class p_a cycle") {
		t.Errorf("mermaid render wrong:\n%s", mer)
	}
}
