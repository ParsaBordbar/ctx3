package flow

import (
	"path/filepath"
	"strings"
	"testing"
)

// fanOutModule is a fixture whose shape is easy to assert on: main calls into
// two packages, one of those calls the other, and one package calls nobody.
func fanOutModule(t *testing.T) string {
	t.Helper()
	td := t.TempDir()
	writeModule(t, td)

	writeGoFile(t, filepath.Join(td, "main.go"), `package main

import (
	"example.com/x/api"
	"example.com/x/store"
)

func main() {
	api.Handle()
	api.Close()
	store.Open()
}
`)
	writeGoFile(t, filepath.Join(td, "api", "api.go"), `package api

import "example.com/x/store"

func Handle() {
	store.Open()
	store.Read()
	helper()
}

func Close() { helper() }

func helper() {}
`)
	writeGoFile(t, filepath.Join(td, "store", "store.go"), `package store

func Open() { seed() }
func Read() {}
func seed() {}
`)
	return td
}

func packageFlow(t *testing.T, dir string) *PackageFlow {
	t.Helper()
	g, err := AnalyzeFlow(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}
	return PackageGraph(g, "x")
}

// find returns the node for a package, failing if the collapse dropped it.
func find(t *testing.T, pf *PackageFlow, name string) PackageNode {
	t.Helper()
	for _, p := range pf.Packages {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("package %q missing from flow map (have %d)", name, len(pf.Packages))
	return PackageNode{}
}

func TestPackageGraph_WeightsAndTargets(t *testing.T) {
	pf := packageFlow(t, fanOutModule(t))

	// main calls api twice (Handle, Close) and store once.
	main := find(t, pf, "main")
	if len(main.Out) != 2 {
		t.Fatalf("main should call 2 packages, got %d: %+v", len(main.Out), main.Out)
	}
	if main.Out[0].To != "api" || main.Out[0].Calls != 2 {
		t.Errorf("heaviest main edge should be api with 2 calls, got %+v", main.Out[0])
	}
	if main.Out[0].Targets != 2 {
		t.Errorf("main should reach 2 distinct api functions, got %d", main.Out[0].Targets)
	}

	// api calls store twice, at two distinct targets.
	api := find(t, pf, "api")
	if len(api.Out) != 1 || api.Out[0].To != "store" || api.Out[0].Calls != 2 {
		t.Errorf("api should have one edge store×2, got %+v", api.Out)
	}
	// api.Handle->helper and api.Close->helper stay inside the package.
	if api.Internal != 2 {
		t.Errorf("api internal calls = %d, want 2", api.Internal)
	}

	// store depends on nothing else, so it is a leaf even though it calls seed.
	store := find(t, pf, "store")
	if len(store.Out) != 0 {
		t.Errorf("store should make no cross-package calls, got %+v", store.Out)
	}
	if store.Internal != 1 {
		t.Errorf("store internal calls = %d, want 1 (Open->seed)", store.Internal)
	}
}

func TestPackageGraph_LeavesAndTotals(t *testing.T) {
	pf := packageFlow(t, fanOutModule(t))

	if got := pf.Leaves(); len(got) != 1 || got[0] != "store" {
		t.Errorf("leaves = %v, want [store]", got)
	}
	if pf.Edges != 3 { // main->api, main->store, api->store
		t.Errorf("edges = %d, want 3", pf.Edges)
	}
	if pf.TotalEntries < 1 {
		t.Errorf("want at least one entry point, got %d", pf.TotalEntries)
	}
	// Every function in the fixture should be accounted for in exactly one package.
	sum := 0
	for _, p := range pf.Packages {
		sum += p.Funcs
	}
	if sum != pf.TotalFuncs {
		t.Errorf("per-package funcs sum to %d but TotalFuncs is %d", sum, pf.TotalFuncs)
	}
}

// The whole point of the package view is that it is drastically smaller than
// the function tree; if that stops being true the view has lost its reason to
// exist.
func TestPackageFlow_IsSmallerThanFunctionTree(t *testing.T) {
	dir := fanOutModule(t)
	g, err := AnalyzeFlow(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("AnalyzeFlow: %v", err)
	}
	tree := RenderText(g)
	pkgs := RenderPackageFlow(PackageGraph(g, "x"))
	if len(pkgs) >= len(tree) {
		t.Errorf("package view (%d bytes) should be smaller than the tree (%d bytes)", len(pkgs), len(tree))
	}
}

func TestRenderPackageFlow_ShowsEdgesAndLeaves(t *testing.T) {
	out := RenderPackageFlow(packageFlow(t, fanOutModule(t)))

	for _, want := range []string{"Flow map — x", "main", "api", "store", "cross-package"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendering missing %q:\n%s", want, out)
		}
	}
	// Entry packages are flagged so the reader knows where execution starts.
	if !strings.Contains(out, "🚀") {
		t.Errorf("rendering should mark entry packages:\n%s", out)
	}
	// Equal-weight targets share a row rather than repeating the arrow.
	if strings.Count(out, "──▶") > 4 {
		t.Errorf("expected equal-weight edges to be grouped, got:\n%s", out)
	}
}

func TestRenderPackageMermaid_IsValidAndWeighted(t *testing.T) {
	out := RenderPackageMermaid(packageFlow(t, fanOutModule(t)))

	if !strings.HasPrefix(out, "```mermaid\nflowchart LR\n") {
		t.Errorf("want a mermaid flowchart, got:\n%s", out)
	}
	if !strings.Contains(out, "-->|2| api") {
		t.Errorf("edges should carry their call-site weight:\n%s", out)
	}
	// Entry packages use the stadium shape.
	if !strings.Contains(out, `main(["main`) {
		t.Errorf("entry package should be a stadium node:\n%s", out)
	}
	if !strings.HasSuffix(out, "```\n") {
		t.Errorf("fence should be closed:\n%s", out)
	}
}

// A one-function package must not read as "1 funcs".
func TestPackageFlow_SingularCounts(t *testing.T) {
	out := RenderPackageFlow(packageFlow(t, fanOutModule(t)))
	if strings.Contains(out, "1 funcs") || strings.Contains(out, "1 entries") {
		t.Errorf("singular counts should not be pluralized:\n%s", out)
	}
}

func TestPackageGraph_NilAndEmpty(t *testing.T) {
	if pf := PackageGraph(nil, "x"); pf == nil || len(pf.Packages) != 0 {
		t.Errorf("nil graph should give an empty flow, got %+v", pf)
	}
	out := RenderPackageFlow(PackageGraph(&CallGraph{}, "x"))
	if !strings.Contains(out, "No calls resolved") {
		t.Errorf("empty graph should explain itself, got:\n%s", out)
	}
}
