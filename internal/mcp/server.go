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
	"strings"

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
			Description: "Create a fully-formed story (a markdown intent file): title plus goal/acceptance body, owner, scope paths, epic, priority. Returns the id, the suggested branch glob, and the commit trailer to use. Statuses are always derived from git — there is no way to set one.",
			InputSchema: objSchema(map[string]any{
				"title":    map[string]any{"type": "string", "description": "One-line title of the unit of work"},
				"body":     map[string]any{"type": "string", "description": "Markdown body — a '## Goal' section and a '## Acceptance' checklist. Omit for a placeholder template."},
				"owner":    map[string]any{"type": "string", "description": "Who owns this spec"},
				"paths":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Declared scope globs (e.g. src/auth/**); work mostly outside them is reported as scope creep"},
				"epic":     map[string]any{"type": "string", "description": "Backlog grouping slug (e.g. user-auth)"},
				"priority": map[string]any{"type": "number", "description": "1=now, 2=next, 3=later"},
				"repo":     repoProp,
			}, "title"),
		},
		{
			Name:        "update_spec",
			Description: "Adjust an existing story's intent: title, body, owner, branch glob, paths, epic, priority — any subset. Writes the markdown file (a plain git diff). Status is not an intent field and cannot be set, here or anywhere.",
			InputSchema: objSchema(map[string]any{
				"id":       map[string]any{"type": "string", "description": "Spec id, e.g. tb-4f2a"},
				"title":    map[string]any{"type": "string"},
				"body":     map[string]any{"type": "string", "description": "Full replacement markdown body"},
				"owner":    map[string]any{"type": "string"},
				"branch":   map[string]any{"type": "string", "description": "Branch glob to link"},
				"paths":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"epic":     map[string]any{"type": "string"},
				"priority": map[string]any{"type": "number"},
				"repo":     repoProp,
			}, "id"),
		},
		{
			Name:        "get_board",
			Description: "Get the full derived board as JSON: spec statuses, branch units, drift report (stale promises, shadow work, scope creep, regressions), and digest. Read-only.",
			InputSchema: objSchema(map[string]any{"repo": repoProp}),
		},
	}
}

// strictArgs unmarshals tool arguments rejecting unknown fields — so an
// attempt to pass e.g. "status" fails loudly instead of being silently
// dropped. Statuses are derived; the API surface must say so.
func strictArgs(args json.RawMessage, into any) error {
	if len(args) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return fmt.Errorf("%v — note: intent fields only; statuses are derived from git and cannot be set", err)
		}
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func orDefault(repo, def string) string {
	if repo == "" {
		return def
	}
	return repo
}

func callTool(name string, args json.RawMessage, defaultRepo string) (string, error) {
	switch name {
	case "list_specs", "get_board":
		var a struct {
			Repo string `json:"repo"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		res, err := audit.Audit(orDefault(a.Repo, defaultRepo), audit.Options{})
		if err != nil {
			return "", err
		}
		if name == "list_specs" {
			return marshal(res.Specs)
		}
		return marshal(res)

	case "get_brief":
		var a struct {
			Repo string `json:"repo"`
			ID   string `json:"id"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		if a.ID == "" {
			return "", fmt.Errorf("get_brief requires an id")
		}
		return audit.Brief(orDefault(a.Repo, defaultRepo), a.ID)

	case "create_spec":
		var a struct {
			Repo     string   `json:"repo"`
			Title    string   `json:"title"`
			Body     string   `json:"body"`
			Owner    string   `json:"owner"`
			Paths    []string `json:"paths"`
			Epic     string   `json:"epic"`
			Priority int      `json:"priority"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		if a.Title == "" {
			return "", fmt.Errorf("create_spec requires a title")
		}
		s, err := spec.New(orDefault(a.Repo, defaultRepo), a.Title, a.Owner)
		if err != nil {
			return "", err
		}
		if a.Body != "" {
			s.Body = a.Body
		}
		s.Paths, s.Epic, s.Priority = a.Paths, a.Epic, a.Priority
		if err := s.Save(); err != nil {
			return "", err
		}
		return marshal(map[string]string{
			"id":      s.ID,
			"file":    s.File,
			"branch":  s.Branch,
			"trailer": s.Trailer(),
			"next":    "work on a matching branch with the trailer in every commit; the board derives the rest",
		})

	case "update_spec":
		var a struct {
			Repo     string    `json:"repo"`
			ID       string    `json:"id"`
			Title    *string   `json:"title"`
			Body     *string   `json:"body"`
			Owner    *string   `json:"owner"`
			Branch   *string   `json:"branch"`
			Paths    *[]string `json:"paths"`
			Epic     *string   `json:"epic"`
			Priority *int      `json:"priority"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		repo := orDefault(a.Repo, defaultRepo)
		s, err := spec.Find(repo, a.ID)
		if err != nil {
			return "", describeUnknownSpec(repo, a.ID)
		}
		apply := func(dst *string, v *string) {
			if v != nil {
				*dst = *v
			}
		}
		apply(&s.Title, a.Title)
		apply(&s.Body, a.Body)
		apply(&s.Owner, a.Owner)
		apply(&s.Branch, a.Branch)
		apply(&s.Epic, a.Epic)
		if a.Paths != nil {
			s.Paths = *a.Paths
		}
		if a.Priority != nil {
			s.Priority = *a.Priority
		}
		if err := s.Save(); err != nil {
			return "", err
		}
		return marshal(map[string]string{"id": s.ID, "file": s.File, "result": "intent updated — status stays derived"})

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// describeUnknownSpec turns a failed lookup into an actionable error by
// listing the ids that do exist.
func describeUnknownSpec(repo, id string) error {
	specs, err := spec.Load(repo)
	if err != nil || len(specs) == 0 {
		return fmt.Errorf("no spec with id %q (no specs found in this repo — create one with create_spec)", id)
	}
	ids := make([]string, len(specs))
	for i, s := range specs {
		ids[i] = s.ID
	}
	return fmt.Errorf("no spec with id %q — known ids: %s", id, strings.Join(ids, ", "))
}

func marshal(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	return string(b), err
}
