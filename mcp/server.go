// Package mcp exposes ctx3's read-only code-analysis facts over the Model
// Context Protocol so a coding agent can query them live, mid-task, instead of
// reading frozen skill snapshots. The transport is newline-delimited JSON-RPC
// 2.0 over stdio — the framing Claude Code uses to launch a local MCP server.
//
// This is a deliberately small, dependency-free implementation: one read loop,
// a handful of JSON-RPC methods (initialize, tools/list, tools/call), and a
// tool registry (tools.go) that wraps the existing analysis packages. No
// mutation is exposed.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"github.com/parsabordbar/ctx3/internal/version"
)

// protocolVersion is the MCP revision this server implements. When a client
// requests a specific version at initialize time we echo theirs; this is the
// fallback for clients that omit it.
const protocolVersion = "2025-06-18"

// request is an incoming JSON-RPC 2.0 message. A message with no ID is a
// notification and gets no response.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// response is an outgoing JSON-RPC 2.0 result or error. Exactly one of Result
// or Error is set.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes used here (subset of the spec).
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInternalError  = -32603
)

// Server runs the JSON-RPC read loop over the given streams.
type Server struct {
	in       io.Reader
	out      io.Writer
	tools    map[string]Tool
	order    []string // stable tools/list order
	writeErr error    // first write failure; ends the read loop
}

// NewServer wires a server over in/out with the default read-only tool set.
func NewServer(in io.Reader, out io.Writer) *Server {
	s := &Server{in: in, out: out, tools: map[string]Tool{}}
	for _, t := range defaultTools() {
		s.tools[t.Name] = t
		s.order = append(s.order, t.Name)
	}
	return s
}

// Serve reads newline-delimited JSON-RPC messages until EOF, dispatching each.
// A parse or dispatch failure on one message is reported back as a JSON-RPC
// error (when the message had an ID) and never tears down the loop.
func (s *Server) Serve() error {
	sc := bufio.NewScanner(s.in)
	// Analysis payloads (whole call graphs, symbol indexes) can be large; a
	// single request line is small, but be generous rather than truncate.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.write(response{JSONRPC: "2.0", Error: &rpcError{codeParse, "parse error: " + err.Error()}})
		} else {
			s.dispatch(req)
		}
		if s.writeErr != nil {
			return fmt.Errorf("writing response: %w", s.writeErr)
		}
	}
	return sc.Err()
}

// dispatch routes one request. Notifications (no ID) never get a response.
func (s *Server) dispatch(req request) {
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.reply(req, s.handleInitialize(req.Params))
	case "notifications/initialized", "notifications/cancelled":
		// Acknowledged silently — nothing to do.
	case "ping":
		s.reply(req, struct{}{})
	case "tools/list":
		s.reply(req, s.handleToolsList())
	case "tools/call":
		result, rpcErr := s.handleToolsCall(req.Params)
		if rpcErr != nil {
			s.write(response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
			return
		}
		s.reply(req, result)
	default:
		if !isNotification {
			s.write(response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{codeMethodNotFound, "unknown method: " + req.Method}})
		}
	}
}

// reply writes a successful result unless the request was a notification.
func (s *Server) reply(req request, result any) {
	if len(req.ID) == 0 {
		return
	}
	s.write(response{JSONRPC: "2.0", ID: req.ID, Result: result})
}

// write emits one response line. A write failure means the client is gone
// (closed stdout / broken pipe), so it is recorded and ends the read loop
// rather than leaving the server spinning on a stream nobody reads.
func (s *Server) write(resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		// Last-resort: a result that would not marshal becomes an internal error.
		b, _ = json.Marshal(response{JSONRPC: "2.0", ID: resp.ID, Error: &rpcError{codeInternalError, "marshal failed"}})
	}
	b = append(b, '\n')
	if _, err := s.out.Write(b); err != nil && s.writeErr == nil {
		s.writeErr = err
	}
}

// handleInitialize advertises capabilities and server identity. It echoes the
// client's requested protocolVersion when present.
func (s *Server) handleInitialize(params json.RawMessage) any {
	ver := protocolVersion
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 && json.Unmarshal(params, &p) == nil && p.ProtocolVersion != "" {
		ver = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": ver,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo": map[string]any{
			"name":    "ctx3",
			"title":   "ctx3 code analysis",
			"version": version.Version(),
		},
		"instructions": instructions,
	}
}

// instructions tell the client how to get the most out of the tool set. They
// are surfaced to the model, so they describe when to reach for which tool and
// how to keep answers small rather than restating the schemas.
const instructions = `ctx3 answers structural questions about a codebase without reading files.

Pick the narrowest tool for the question:
  ctx3_context    orient on an unfamiliar repo (counts, deps, languages, README)
  ctx3_map        where is a symbol defined / what does a package export
  ctx3_functions  a function's exact signature, params and results
  ctx3_deps       which packages import which, and any import cycles
  ctx3_flow       what calls what, and where execution starts
  ctx3_impact     what breaks if I change this symbol
  ctx3_db         what databases and schema the project has

All tools are read-only and answer live against the current working tree, so
re-call after edits instead of trusting an earlier answer.

Results default to a compact text rendering; pass format:"json" only when you
need to consume the structure programmatically, as it costs far more tokens.
Results are truncated past maxBytes — if you see a truncation marker, narrow
with dir/match/kind/depth/entryOnly rather than raising the cap.

ctx3_flow and ctx3_impact type-check the module and need a compiling tree; the
other tools are parse-only and work on partial or broken code.`

// handleToolsList returns the registered tools in stable registration order.
// Every tool is annotated read-only and idempotent: ctx3 only ever reads the
// tree, so a client is free to call these without confirmation or to retry one.
func (s *Server) handleToolsList() any {
	list := make([]map[string]any, 0, len(s.order))
	for _, name := range s.order {
		t := s.tools[name]
		list = append(list, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"inputSchema": t.InputSchema,
			"annotations": map[string]any{
				"title":           t.Title,
				"readOnlyHint":    true,
				"idempotentHint":  true,
				"destructiveHint": false,
				"openWorldHint":   false,
			},
		})
	}
	return map[string]any{"tools": list}
}

// handleToolsCall dispatches a tools/call to a registered tool. A tool that
// errors is reported as an MCP tool result with isError=true (not a JSON-RPC
// error) so the agent sees the message and can adjust.
func (s *Server) handleToolsCall(params json.RawMessage) (any, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, &rpcError{codeInvalidRequest, "invalid tools/call params: " + err.Error()}
	}
	tool, ok := s.tools[call.Name]
	if !ok {
		return nil, &rpcError{codeMethodNotFound, "unknown tool: " + call.Name}
	}

	args := map[string]any{}
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return nil, &rpcError{codeInvalidRequest, "invalid arguments: " + err.Error()}
		}
	}

	text, err := tool.Handler(args)
	if err != nil {
		return textResult(fmt.Sprintf("%s failed: %v", tool.Name, err), true), nil
	}
	return textResult(text, false), nil
}

// textResult wraps a string in the MCP content-block result shape.
func textResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}
