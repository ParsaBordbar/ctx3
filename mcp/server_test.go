package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// roundtrip feeds newline-delimited requests through a server and returns the
// decoded responses, keyed by request id (notifications produce no response).
func roundtrip(t *testing.T, requests ...string) map[float64]map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(requests, "\n") + "\n")
	var out strings.Builder
	if err := NewServer(in, &out).Serve(); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	byID := map[float64]map[string]any{}
	sc := bufio.NewScanner(strings.NewReader(out.String()))
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("bad response line %q: %v", sc.Text(), err)
		}
		if id, ok := m["id"].(float64); ok {
			byID[id] = m
		}
	}
	return byID
}

func TestInitializeAndToolsList(t *testing.T) {
	resp := roundtrip(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)

	if len(resp) != 2 {
		t.Fatalf("want 2 responses (notification suppressed), got %d", len(resp))
	}

	init := resp[1]["result"].(map[string]any)
	if got := init["protocolVersion"]; got != "2025-06-18" {
		t.Errorf("protocolVersion = %v, want echoed 2025-06-18", got)
	}
	if init["serverInfo"].(map[string]any)["name"] != "ctx3" {
		t.Error("serverInfo.name != ctx3")
	}
	// Instructions tell the model which tool to reach for; a client that gets
	// none has to guess from descriptions alone.
	if s, _ := init["instructions"].(string); !strings.Contains(s, "ctx3_impact") {
		t.Error("initialize should return instructions covering the tool set")
	}

	tools := resp[2]["result"].(map[string]any)["tools"].([]any)
	got := map[string]map[string]any{}
	for _, tv := range tools {
		t := tv.(map[string]any)
		got[t["name"].(string)] = t
	}
	for _, want := range []string{
		"ctx3_context", "ctx3_map", "ctx3_functions",
		"ctx3_deps", "ctx3_flow", "ctx3_impact", "ctx3_db",
	} {
		tool, ok := got[want]
		if !ok {
			t.Errorf("tools/list missing %q", want)
			continue
		}
		// Every tool is read-only, and every tool takes the common args that
		// keep a result small enough to be worth reading.
		if ann, _ := tool["annotations"].(map[string]any); ann["readOnlyHint"] != true {
			t.Errorf("%s: want readOnlyHint=true, got %v", want, ann["readOnlyHint"])
		}
		props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		for _, arg := range []string{"format", "maxBytes"} {
			if _, ok := props[arg]; !ok {
				t.Errorf("%s: inputSchema missing common arg %q", want, arg)
			}
		}
	}
}

// callText runs one tools/call and returns its text content, failing on an
// isError result.
func callText(t *testing.T, name, args string) string {
	t.Helper()
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`
	result := roundtrip(t, req)[1]["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("%s errored: %v", name, result)
	}
	return result["content"].([]any)[0].(map[string]any)["text"].(string)
}

// The parse-only tools work against this package without a build, so they can
// be exercised for real rather than mocked.
func TestParseOnlyToolsAnswerLive(t *testing.T) {
	if got := callText(t, "ctx3_map", `{"dir":".","all":true}`); !strings.Contains(got, "NewServer") {
		t.Errorf("ctx3_map should index NewServer; got:\n%s", got)
	}
	if got := callText(t, "ctx3_functions", `{"dir":".","all":true,"match":"^NewServer$"}`); !strings.Contains(got, "func NewServer") {
		t.Errorf("ctx3_functions should show the NewServer signature; got:\n%s", got)
	}
	if got := callText(t, "ctx3_context", `{"dir":"."}`); !strings.Contains(got, "Files:") {
		t.Errorf("ctx3_context should report file counts; got:\n%s", got)
	}
	// No database in this tree — the tool should say so, not fail.
	if got := callText(t, "ctx3_db", `{"dir":"."}`); !strings.Contains(got, "Databases") {
		t.Errorf("ctx3_db should render a report; got:\n%s", got)
	}
}

// ctx3_flow defaults to the package map: the function tree runs to tens of
// thousands of tokens on a real repo, so the cheap orienting view is the one a
// caller gets without asking.
func TestFlowDefaultsToPackageView(t *testing.T) {
	pkgs := callText(t, "ctx3_flow", `{"dir":".."}`)
	if !strings.Contains(pkgs, "Flow map") {
		t.Fatalf("default flow view should be the package map, got:\n%.300s", pkgs)
	}

	fns := callText(t, "ctx3_flow", `{"dir":"..","view":"functions","maxBytes":9999999}`)
	if !strings.Contains(fns, "Code Flow") {
		t.Errorf(`view:"functions" should render the call tree, got:\n%.300s`, fns)
	}
	if len(pkgs) >= len(fns) {
		t.Errorf("package view (%d bytes) should be far smaller than the function tree (%d bytes)", len(pkgs), len(fns))
	}

	// An unknown view is rejected rather than silently falling back, so a typo
	// can't be mistaken for a deliberate choice of view.
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctx3_flow","arguments":{"view":"nope"}}}`
	bad := roundtrip(t, req)[1]["result"].(map[string]any)
	if bad["isError"] != true {
		t.Error("unknown view should be an error")
	}
}

// Text is the default because JSON costs several times the tokens for the same
// facts; both must stay reachable.
func TestFormatArg(t *testing.T) {
	text := callText(t, "ctx3_map", `{"dir":"."}`)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		t.Error("default format should be text, got JSON")
	}
	asJSON := callText(t, "ctx3_map", `{"dir":".","format":"json"}`)
	if !strings.HasPrefix(strings.TrimSpace(asJSON), "{") {
		t.Errorf("format=json should return a JSON object, got:\n%.120s", asJSON)
	}
	if len(asJSON) <= len(text) {
		t.Errorf("expected JSON (%d bytes) to be larger than text (%d bytes)", len(asJSON), len(text))
	}
}

// An unbounded result would swamp the caller's context, so a large answer is
// cut and labelled rather than returned whole.
func TestTruncation(t *testing.T) {
	// ".." is the whole repo — a payload comfortably over the cap under test.
	got := callText(t, "ctx3_map", `{"dir":"..","maxBytes":500}`)
	if !strings.Contains(got, "[truncated:") {
		t.Fatalf("want a truncation marker, got:\n%s", got)
	}
	if len(got) > 500+300 { // payload cut at 500; the marker itself is short
		t.Errorf("truncated result is %d bytes, want ~500 plus a marker", len(got))
	}
	full := callText(t, "ctx3_map", `{"dir":".."}`)
	if strings.Contains(full, "[truncated:") {
		t.Error("default maxBytes should not truncate this repo's symbol index")
	}
}

func TestErrorPaths(t *testing.T) {
	resp := roundtrip(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctx3_impact","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"nonsense/method"}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ctx3_map","arguments":{"match":"[unclosed"}}}`,
	)

	// Missing required arg -> tool result with isError true (not a JSON-RPC error).
	r1 := resp[1]["result"].(map[string]any)
	if r1["isError"] != true {
		t.Error("missing symbol should yield isError=true tool result")
	}
	// Unknown tool / unknown method -> JSON-RPC error object.
	if _, ok := resp[2]["error"]; !ok {
		t.Error("unknown tool should yield a JSON-RPC error")
	}
	if _, ok := resp[3]["error"]; !ok {
		t.Error("unknown method should yield a JSON-RPC error")
	}
	// A bad filter is reported, not silently dropped — a silently ignored
	// regexp would return a complete index the caller reads as filtered.
	r4 := resp[4]["result"].(map[string]any)
	if r4["isError"] != true {
		t.Error("invalid match regexp should yield isError=true")
	}
	if msg := r4["content"].([]any)[0].(map[string]any)["text"].(string); !strings.Contains(msg, "match") {
		t.Errorf("error should name the bad argument, got %q", msg)
	}
}

// A client that goes away mid-session closes the pipe; the server must stop
// rather than keep serving a stream nobody reads.
func TestWriteFailureEndsLoop(t *testing.T) {
	in := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")
	err := NewServer(in, errWriter{}).Serve()
	if err == nil {
		t.Fatal("want an error when the output stream fails")
	}
	if !strings.Contains(err.Error(), "writing response") {
		t.Errorf("error should identify the write failure, got %v", err)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// --- tools added alongside the analysis wrappers ---

func TestTreeToolRespectsDepth(t *testing.T) {
	shallow := callText(t, "ctx3_tree", `{"dir":"..","depth":1}`)
	if !strings.Contains(shallow, "mcp") || !strings.Contains(shallow, "symbols") {
		t.Errorf("depth 1 should list the top-level packages:\n%s", shallow)
	}
	// A file two levels down must not appear.
	if strings.Contains(shallow, "server.go") {
		t.Errorf("depth 1 leaked a nested file:\n%s", shallow)
	}
	if deep := callText(t, "ctx3_tree", `{"dir":"..","depth":2}`); !strings.Contains(deep, "server.go") {
		t.Errorf("depth 2 should reach server.go:\n%.400s", deep)
	}
}

// The pack tool defaults to the tree alone. A caller asking for "the repo"
// without narrowing it should get something that fits, not a truncated dump.
func TestPackToolDefaultsToStructureOnly(t *testing.T) {
	got := callText(t, "ctx3_pack", `{"dir":"."}`)
	if !strings.Contains(got, "Directory structure") {
		t.Errorf("expected the tree section:\n%s", got)
	}
	if strings.Contains(got, "package mcp") {
		t.Errorf("structure-only pack leaked file contents:\n%.400s", got)
	}
}

func TestPackToolFilesSectionWithIncludeGlob(t *testing.T) {
	got := callText(t, "ctx3_pack", `{"dir":".","section":"files","include":"tools.go"}`)
	if !strings.Contains(got, "package mcp") {
		t.Errorf("expected tools.go contents:\n%.400s", got)
	}
	if strings.Contains(got, "server_test.go") {
		t.Errorf("include glob was not applied:\n%.400s", got)
	}
}

func TestPackToolRejectsUnknownStyleAndSection(t *testing.T) {
	for _, args := range []string{
		`{"dir":".","style":"yaml"}`,
		`{"dir":".","section":"everything"}`,
	} {
		resp := roundtrip(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ctx3_pack","arguments":`+args+`}}`)
		result := resp[1]["result"].(map[string]any)
		if result["isError"] != true {
			t.Errorf("%s should be an error, got %v", args, result)
		}
	}
}

func TestGitToolReportsRepositoryState(t *testing.T) {
	if brief := callText(t, "ctx3_brief", `{"dir":"..","query":"Impacted","max":2,"budget":800}`); !strings.Contains(brief, "flow.Impacted") {
		t.Fatalf("ctx3_brief should find flow.Impacted:\n%s", brief)
	}
	if dc := callText(t, "ctx3_diff_context", `{"dir":"..","budget":600}`); !strings.Contains(dc, "Change context") {
		t.Fatalf("ctx3_diff_context header missing:\n%s", dc)
	}
	got := callText(t, "ctx3_git", `{"dir":"..","commits":3}`)
	if !strings.Contains(got, "Repository") {
		t.Fatalf("expected repository header:\n%.400s", got)
	}
	// ctx3 is itself a checkout, so real history has to come back.
	if !strings.Contains(got, "Hot files") && !strings.Contains(got, "Recent commits") {
		t.Errorf("expected history facts:\n%.600s", got)
	}
}
