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
	"path/filepath"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
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
//
// A repository that cannot be read fails here, before the handshake. The
// alternative is a server that starts happily and answers every single
// tool call with the same error — which an agent reads as "the board is
// broken", not as "you pointed me at the wrong directory".
func Serve(in io.Reader, out io.Writer, defaultRepo, version string) error {
	if _, err := gitrepo.Run(defaultRepo, "rev-parse", "--git-dir"); err != nil {
		return err
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)
	// One probe for the life of the server: this process's own build cannot
	// change under it, so the answer cannot either.
	var stale staleness
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue // not a parseable frame; nothing to respond to
		}
		if resp := handle(req, defaultRepo, version, stale.warning); resp != nil {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// handle answers one frame. staleWarning is asked, for status-bearing tools
// only, whether this build has been superseded — a func rather than a string
// so the probe it costs is never paid by a session that only writes intent.
func handle(req request, defaultRepo, version string, staleWarning func(string) string) *response {
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
		// A second content block, never a prefix: the first block is JSON for
		// get_board and list_specs, and a banner glued to the front of it is
		// a board no client can parse.
		content := []map[string]any{{"type": "text", "text": text}}
		if statusBearing[p.Name] && staleWarning != nil {
			if warn := staleWarning(version); warn != "" {
				content = append(content, map[string]any{"type": "text", "text": warn})
			}
		}
		resp.Result = map[string]any{"content": content}
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
				"sprint":   map[string]any{"type": "string", "description": "Iteration slug (e.g. s12, 2026-29) — intent, never a status"},
				"priority": map[string]any{"type": "number", "description": "1=now, 2=next, 3=later"},
				"points":   map[string]any{"type": "number", "description": "Story-point estimate; omit for unestimated"},
				"type":     map[string]any{"type": "string", "description": "story | bug | task (default story)"},
				"needs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Spec ids that must be done before this starts; readiness is derived"},
				"hold":     map[string]any{"type": "string", "description": "Why work is paused, in one human sentence. Intent, not a status — git still derives the status, and contradicts the note when the work lands or resumes"},
				"repos":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Workspace repos this story must land in (\"hub\" or spoke names); done requires the trailer landed in every one"},
				"repo":     repoProp,
			}, "title"),
		},
		{
			Name:        "update_spec",
			Description: "Adjust an existing story's intent: title, body, owner, branch glob, paths, epic, sprint, priority — any subset. Writes the markdown file (a plain git diff). Status is not an intent field and cannot be set, here or anywhere.",
			InputSchema: objSchema(map[string]any{
				"id":       map[string]any{"type": "string", "description": "Spec id, e.g. tb-4f2a"},
				"title":    map[string]any{"type": "string"},
				"body":     map[string]any{"type": "string", "description": "Full replacement markdown body"},
				"owner":    map[string]any{"type": "string"},
				"branch":   map[string]any{"type": "string", "description": "Branch glob to link"},
				"paths":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"epic":     map[string]any{"type": "string"},
				"sprint":   map[string]any{"type": "string", "description": "Iteration slug; empty string clears it"},
				"priority": map[string]any{"type": "number"},
				"points":   map[string]any{"type": "number", "description": "Story-point estimate; 0 clears it"},
				"type":     map[string]any{"type": "string", "description": "story | bug | task; empty string resets to story"},
				"needs":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Full replacement needs list; empty array clears it"},
				"hold":     map[string]any{"type": "string", "description": "Why work is paused; empty string clears it — there is no unhold verb"},
				"repos":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Full replacement repos list (\"hub\" or spoke names); empty array clears it"},
				"repo":     repoProp,
			}, "id"),
		},
		{
			Name:        "delete_spec",
			Description: "Retire a story by deleting its intent file — for one created by mistake. Refused while git still points at it (a linked branch, or a landed commit carrying its trailer), because deleting the promise would leave that work unexplained; pass force to retire it anyway. The deletion is a commit, so the undo is git revert.",
			InputSchema: objSchema(map[string]any{
				"id":    map[string]any{"type": "string", "description": "Spec id, e.g. tb-4f2a"},
				"force": map[string]any{"type": "boolean", "description": "Delete even though git still references the story"},
				"repo":  repoProp,
			}, "id"),
		},
		{
			Name:        "check_acceptance",
			Description: "Record that acceptance criteria came true: ticks (or unticks) them in the story file, editing only those checkbox lines. Call it as each criterion becomes verifiable — it is part of finishing the work, not paperwork after it, and a landed story whose criteria are unticked is reported as drift. Never tick what you have not verified. This is a claim, not a status: it cannot set, block or change what git derives.",
			InputSchema: objSchema(map[string]any{
				"id": map[string]any{"type": "string", "description": "Spec id, e.g. tb-4f2a"},
				"criteria": map[string]any{
					"type": "array", "items": map[string]any{"type": "string"},
					"description": "Which criteria: 1-based indices (\"2\"), a unique substring of a criterion's text, or \"all\". Anything ambiguous fails and returns the numbered checklist.",
				},
				"uncheck": map[string]any{"type": "boolean", "description": "Untick instead — a criterion that stopped being true"},
				"repo":    repoProp,
			}, "id", "criteria"),
		},
		{
			Name:        "next_spec",
			Description: "The story an idle agent should pick up: the highest-priority planned spec (planned = no branch or commit yet, so unclaimed), returned as the same ready-to-work packet as get_brief. Deterministic — same repo state, same answer. When nothing is planned it says so; never invents work.",
			InputSchema: objSchema(map[string]any{"repo": repoProp}),
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

	case "check_acceptance":
		var a struct {
			Repo     string   `json:"repo"`
			ID       string   `json:"id"`
			Criteria []string `json:"criteria"`
			Uncheck  bool     `json:"uncheck"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		if a.ID == "" {
			return "", fmt.Errorf("check_acceptance requires an id")
		}
		s, err := spec.Find(orDefault(a.Repo, defaultRepo), a.ID)
		if err != nil {
			return "", err
		}
		changed, err := s.SetAcceptance(a.Criteria, !a.Uncheck)
		if err != nil {
			return "", err
		}
		verb := "ticked"
		if a.Uncheck {
			verb = "unticked"
		}
		done, total := spec.Progress(s.Body)
		if len(changed) == 0 {
			return fmt.Sprintf("No change — already %s. %s is at %d/%d criteria.", verb, s.ID, done, total), nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s %d criterion(s) on %s — now %d/%d:\n\n", verb, len(changed), s.ID, done, total)
		for _, c := range changed {
			fmt.Fprintf(&b, "  %d. %s\n", c.N, c.Text)
		}
		// The tick lives in a file; a file only becomes evidence once it is
		// committed, and the trailer is what carries it onto the board.
		fmt.Fprintf(&b, "\nCommit %s with the trailer %q so the sign-off travels with the work. The status stays git's.",
			filepath.Base(s.File), s.Trailer())
		return b.String(), nil

	case "next_spec":
		var a struct {
			Repo string `json:"repo"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		repo := orDefault(a.Repo, defaultRepo)
		up, err := audit.Next(repo)
		if err != nil {
			return "", err
		}
		// The reminder leads: an agent that landed work without ticking its
		// acceptance is about to do it again, and this is the one moment it
		// is still holding the context to fix it.
		reminder := audit.SignoffReminder(up.Unverified)
		if reminder != "" {
			reminder += "\n\n"
		}
		if up.Spec == nil {
			// An empty backlog is an answer, not an error — and it must
			// not tempt the caller into inventing work.
			msg := "Nothing is startable — every story has work in flight or landed."
			for _, w := range up.Waiting {
				msg += fmt.Sprintf(" %s waits on %s.", w.ID, strings.Join(w.Waiting, ", "))
			}
			if up.Stalled > 0 {
				msg += fmt.Sprintf(" %d stalled stories may be worth resuming (see get_board).", up.Stalled)
			}
			return reminder + msg + " If you have new intent, create_spec it; do not invent work.", nil
		}
		pri := ""
		if up.Spec.Priority > 0 {
			pri = fmt.Sprintf(" (priority %d)", up.Spec.Priority)
		}
		brief, err := audit.Brief(repo, up.Spec.ID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%sNext up: %s — %s%s\n\n%s", reminder, up.Spec.ID, up.Spec.Title, pri, brief), nil

	case "create_spec":
		var a struct {
			Repo     string   `json:"repo"`
			Title    string   `json:"title"`
			Body     string   `json:"body"`
			Owner    string   `json:"owner"`
			Paths    []string `json:"paths"`
			Epic     string   `json:"epic"`
			Sprint   string   `json:"sprint"`
			Priority int      `json:"priority"`
			Points   int      `json:"points"`
			Type     string   `json:"type"`
			Needs    []string `json:"needs"`
			Hold     string   `json:"hold"`
			Repos    []string `json:"repos"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		if a.Title == "" {
			return "", fmt.Errorf("create_spec requires a title")
		}
		// Validate before creating, so a bad argument never leaves an
		// orphan spec file behind.
		if !spec.ValidType(a.Type) {
			return "", spec.ErrType(a.Type)
		}
		if err := spec.ValidateNeeds(orDefault(a.Repo, defaultRepo), a.Needs, ""); err != nil {
			return "", err
		}
		if err := spec.ValidateRepos(orDefault(a.Repo, defaultRepo), a.Repos); err != nil {
			return "", err
		}
		s, err := spec.New(orDefault(a.Repo, defaultRepo), a.Title, a.Owner)
		if err != nil {
			return "", err
		}
		if a.Body != "" {
			s.Body = a.Body
		}
		s.Paths, s.Epic, s.Sprint, s.Priority, s.Points, s.Type, s.Needs, s.Hold, s.Repos = a.Paths, a.Epic, a.Sprint, a.Priority, a.Points, a.Type, a.Needs, a.Hold, a.Repos
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

	case "delete_spec":
		var a struct {
			Repo  string `json:"repo"`
			ID    string `json:"id"`
			Force bool   `json:"force"`
		}
		if err := strictArgs(args, &a); err != nil {
			return "", err
		}
		repo := orDefault(a.Repo, defaultRepo)
		if _, err := spec.Find(repo, a.ID); err != nil {
			return "", describeUnknownSpec(repo, a.ID)
		}
		if !a.Force {
			refs, err := audit.Referencing(repo, a.ID)
			if err != nil {
				return "", err
			}
			if len(refs) > 0 {
				return "", fmt.Errorf("%s still has proof in git — %s. Deleting the story would leave that work unexplained; pass force: true to retire it anyway",
					a.ID, strings.Join(refs, ", "))
			}
		}
		s, err := spec.Delete(repo, a.ID)
		if err != nil {
			return "", err
		}
		return marshal(map[string]any{
			"id":      s.ID,
			"deleted": true,
			"file":    s.File,
			"next":    "commit the deletion like any intent change; the undo is git revert",
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
			Sprint   *string   `json:"sprint"`
			Priority *int      `json:"priority"`
			Points   *int      `json:"points"`
			Type     *string   `json:"type"`
			Needs    *[]string `json:"needs"`
			Hold     *string   `json:"hold"`
			Repos    *[]string `json:"repos"`
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
		apply(&s.Sprint, a.Sprint)
		apply(&s.Hold, a.Hold)
		if a.Paths != nil {
			s.Paths = *a.Paths
		}
		if a.Priority != nil {
			s.Priority = *a.Priority
		}
		if a.Points != nil {
			s.Points = *a.Points
		}
		if a.Type != nil {
			if !spec.ValidType(*a.Type) {
				return "", spec.ErrType(*a.Type)
			}
			s.Type = *a.Type
		}
		if a.Needs != nil {
			if err := spec.ValidateNeeds(repo, *a.Needs, s.ID); err != nil {
				return "", err
			}
			s.Needs = *a.Needs
		}
		if a.Repos != nil {
			if err := spec.ValidateRepos(repo, *a.Repos); err != nil {
				return "", err
			}
			s.Repos = *a.Repos
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
