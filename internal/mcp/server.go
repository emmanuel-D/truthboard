// Package mcp exposes the spec layer over the Model Context Protocol
// (stdio transport, newline-delimited JSON-RPC 2.0) so agents like Claude
// Code stop shelling out. Specs are the only writable surface — by design
// there is no tool that sets a status, because statuses are derived.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// Serve runs the MCP loop until stdin closes. defaultRepo is used when a
// tool call omits the repo argument (Claude Code runs servers in the
// project directory, so "." is the right default).
func Serve(in io.Reader, out io.Writer, defaultRepo, version string) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue // not a parseable frame; nothing to respond to
		}
		if resp := handle(req, defaultRepo, version); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func handle(req request, defaultRepo, version string) *response {
	if req.ID == nil {
		return nil // notification — never answered
	}
	resp := &response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2024-11-05"
		}
		resp.Result = map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "truthboard", "version": version},
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = map[string]any{"tools": tools()}
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			resp.Error = &rpcError{Code: -32602, Message: "invalid params"}
			break
		}
		text, err := callTool(p.Name, p.Arguments, defaultRepo)
		if err != nil {
			resp.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}
			break
		}
		resp.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
		}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

func objSchema(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var repoProp = map[string]any{"type": "string", "description": "Repository path (default: current directory)"}

func tools() []toolDef {
	return []toolDef{
		{
			Name:        "list_specs",
			Description: "List all specs with their derived (never typed) statuses.",
			InputSchema: objSchema(map[string]any{"repo": repoProp}),
		},
		{
			Name:        "get_brief",
			Description: "Get the full context packet for one spec: intent, acceptance criteria, branch/trailer linking instructions, current derived status. Call this before starting work on a spec.",
			InputSchema: objSchema(map[string]any{
				"id":   map[string]any{"type": "string", "description": "Spec id, e.g. tb-4f2a"},
				"repo": repoProp,
			}, "id"),
		},
		{
			Name:        "create_spec",
			Description: "Create a new spec (a markdown intent file). Returns the id, the suggested branch glob, and the commit trailer to use. Statuses are always derived from git — there is no way to set one.",
			InputSchema: objSchema(map[string]any{
				"title": map[string]any{"type": "string", "description": "One-line title of the unit of work"},
				"owner": map[string]any{"type": "string", "description": "Who owns this spec"},
				"repo":  repoProp,
			}, "title"),
		},
		{
			Name:        "get_board",
			Description: "Get the full derived board as JSON: spec statuses, branch units, drift report (stale promises, shadow work, scope creep, regressions), and digest. Read-only.",
			InputSchema: objSchema(map[string]any{"repo": repoProp}),
		},
	}
}

func callTool(name string, args json.RawMessage, defaultRepo string) (string, error) {
	var a struct {
		Repo  string `json:"repo"`
		ID    string `json:"id"`
		Title string `json:"title"`
		Owner string `json:"owner"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if a.Repo == "" {
		a.Repo = defaultRepo
	}

	switch name {
	case "list_specs":
		res, err := audit.Audit(a.Repo, audit.Options{})
		if err != nil {
			return "", err
		}
		return marshal(res.Specs)
	case "get_brief":
		if a.ID == "" {
			return "", fmt.Errorf("get_brief requires an id")
		}
		return audit.Brief(a.Repo, a.ID)
	case "create_spec":
		if a.Title == "" {
			return "", fmt.Errorf("create_spec requires a title")
		}
		s, err := spec.New(a.Repo, a.Title, a.Owner)
		if err != nil {
			return "", err
		}
		return marshal(map[string]string{
			"id":      s.ID,
			"file":    s.File,
			"branch":  s.Branch,
			"trailer": s.Trailer(),
			"next":    "edit the Goal and Acceptance sections, then work on a matching branch with the trailer in every commit",
		})
	case "get_board":
		res, err := audit.Audit(a.Repo, audit.Options{})
		if err != nil {
			return "", err
		}
		return marshal(res)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

func marshal(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}
