package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "t@t.co"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "chore: init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	specDir := filepath.Join(dir, ".truthboard", "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// priority 1 keeps next_spec deterministic even after tests create
	// more (unprioritized) specs alongside it.
	specMD := "---\nid: tb-mcp1\ntitle: MCP fixture spec\npriority: 1\n---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(specDir, "tb-mcp1-test.md"), []byte(specMD), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// drive sends JSON-RPC frames and returns the decoded responses in order.
func drive(t *testing.T, repo string, frames ...string) []map[string]any {
	t.Helper()
	in := strings.NewReader(strings.Join(frames, "\n") + "\n")
	var out bytes.Buffer
	if err := Serve(in, &out, repo, "test"); err != nil {
		t.Fatal(err)
	}
	var responses []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var r map[string]any
		if err := dec.Decode(&r); err != nil {
			t.Fatal(err)
		}
		responses = append(responses, r)
	}
	return responses
}

func toolText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", resp)
	}
	content := result["content"].([]any)[0].(map[string]any)
	isErr, _ := result["isError"].(bool)
	return content["text"].(string), isErr
}

func TestHandshakeAndToolList(t *testing.T) {
	repo := fixtureRepo(t)
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	)
	if len(responses) != 2 {
		t.Fatalf("got %d responses, want 2 (the notification must not be answered)", len(responses))
	}
	init := responses[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["name"] != "truthboard" {
		t.Errorf("initialize result = %v", init)
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 6 {
		t.Errorf("got %d tools, want 6", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"list_specs", "get_brief", "next_spec", "create_spec", "update_spec", "get_board"} {
		if !names[want] {
			t.Errorf("missing tool %q", want)
		}
	}
}

func TestToolCalls(t *testing.T) {
	repo := fixtureRepo(t)
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_specs","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_brief","arguments":{"id":"tb-mcp1"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Agent-created spec","owner":"claude"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_board","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"set_status","arguments":{"id":"tb-mcp1","status":"done"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"next_spec","arguments":{}}}`,
	)
	if len(responses) != 6 {
		t.Fatalf("got %d responses, want 6", len(responses))
	}

	if text, isErr := toolText(t, responses[0]); isErr || !strings.Contains(text, "tb-mcp1") || !strings.Contains(text, "planned") {
		t.Errorf("list_specs = %.120s (err=%v), want tb-mcp1 planned", text, isErr)
	}
	if text, isErr := toolText(t, responses[1]); isErr || !strings.Contains(text, "Spec: tb-mcp1") {
		t.Errorf("get_brief should include the trailer instruction, got %.120s (err=%v)", text, isErr)
	}
	text, isErr := toolText(t, responses[2])
	if isErr || !strings.Contains(text, "tb-") || !strings.Contains(text, "trailer") {
		t.Errorf("create_spec = %.120s (err=%v)", text, isErr)
	}
	var created struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil || created.File == "" {
		t.Fatalf("create_spec output not parseable: %v", err)
	}
	if _, err := os.Stat(created.File); err != nil {
		t.Errorf("created spec file missing: %v", err)
	}
	if text, isErr := toolText(t, responses[3]); isErr || !strings.Contains(text, `"integration_branch"`) {
		t.Errorf("get_board = %.120s (err=%v)", text, isErr)
	}
	// There is no set_status tool and there never will be.
	if text, isErr := toolText(t, responses[4]); !isErr || !strings.Contains(text, "unknown tool") {
		t.Errorf("set_status must fail as unknown, got %.120s (err=%v)", text, isErr)
	}
	// next_spec hands an idle agent the planned fixture story as a brief.
	if text, isErr := toolText(t, responses[5]); isErr ||
		!strings.Contains(text, "Next up: tb-mcp1") || !strings.Contains(text, "Spec: tb-mcp1") {
		t.Errorf("next_spec = %.120s (err=%v), want tb-mcp1 as a brief", text, isErr)
	}
}

func TestAgentDraftsAndAdjustsStory(t *testing.T) {
	repo := fixtureRepo(t)
	body := "## Goal\\n\\nVerified emails before dashboard access.\\n\\n## Acceptance\\n\\n- [ ] token expires in 24h"
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Email verification","body":"`+body+`","owner":"claude","paths":["src/auth/**"],"epic":"user-auth","priority":1}}}`,
	)
	text, isErr := toolText(t, responses[0])
	if isErr {
		t.Fatalf("create_spec failed: %s", text)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatal(err)
	}

	responses = drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"`+created.ID+`","priority":2,"epic":"onboarding","title":"Email verification at signup"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_specs","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"`+created.ID+`","status":"done"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"tb-nope"}}}`,
	)
	if text, isErr := toolText(t, responses[0]); isErr {
		t.Fatalf("update_spec failed: %s", text)
	}

	// The adjusted intent round-trips through the file.
	raw, err := os.ReadFile(specFileByID(t, repo, created.ID))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	for _, want := range []string{"Email verification at signup", "epic: onboarding", "priority: 2", "token expires in 24h"} {
		if !strings.Contains(content, want) {
			t.Errorf("spec file missing %q after update:\n%s", want, content)
		}
	}

	// Setting a status must fail loudly, not be silently dropped.
	if text, isErr := toolText(t, responses[2]); !isErr || !strings.Contains(text, "derived") {
		t.Errorf("update_spec with status must fail citing derived statuses, got %.150s (err=%v)", text, isErr)
	}
	// Unknown id lists the ids that do exist.
	if text, isErr := toolText(t, responses[3]); !isErr || !strings.Contains(text, created.ID) {
		t.Errorf("unknown-id error should list known ids, got %.150s (err=%v)", text, isErr)
	}
}

func TestSpecTypeIsValidatedNotInvented(t *testing.T) {
	repo := fixtureRepo(t)
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"A bug report","type":"bug"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Nonsense","type":"epic-saga"}}}`,
	)
	text, isErr := toolText(t, responses[0])
	if isErr {
		t.Fatalf("create_spec with type bug failed: %s", text)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(specFileByID(t, repo, created.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "type: bug") {
		t.Errorf("spec file missing type: bug:\n%s", raw)
	}
	if text, isErr := toolText(t, responses[1]); !isErr || !strings.Contains(text, "story, bug, task") {
		t.Errorf("unknown type must fail listing valid values, got %.150s (err=%v)", text, isErr)
	}
}

func specFileByID(t *testing.T, repo, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, ".truthboard", "specs", id+"-*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("spec file for %s: %v %v", id, matches, err)
	}
	return matches[0]
}

// TestAgentDecomposesCrossRepoStory drives the tb-f515 flow end to end
// over MCP: in a workspace, an agent splits a fat story into per-repo
// children under one epic with needs: ordering — and next_spec never hands
// out the blocked half. repos: validation guards the same path.
func TestAgentDecomposesCrossRepoStory(t *testing.T) {
	repo := fixtureRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "repos:\n  api:\n    remote: git@example.com:acme/api.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".truthboard", "workspace.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// The agent narrows the original into the api half…
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"tb-mcp1","title":"Reset flow — api half","repos":["api"],"epic":"reset-flow"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Reset flow — hub half","epic":"reset-flow","needs":["tb-mcp1"],"repos":["hub"],"body":"## Goal\nHub half.\n\n## Acceptance\n\n- [ ] works"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Bad child","repos":["mobile"]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"next_spec","arguments":{}}}`,
	)

	if text, isErr := toolText(t, responses[0]); isErr || !strings.Contains(text, "intent updated") {
		t.Fatalf("update_spec with repos failed: %.200s (err=%v)", text, isErr)
	}
	text, isErr := toolText(t, responses[1])
	if isErr {
		t.Fatalf("create_spec child failed: %.200s", text)
	}
	var child struct {
		ID   string `json:"id"`
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(text), &child); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(child.File)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"epic: reset-flow", "tb-mcp1", "hub"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("child spec missing %q:\n%s", want, raw)
		}
	}

	// Unknown repo refused before any file exists.
	if text, isErr := toolText(t, responses[2]); !isErr || !strings.Contains(text, "known repos: hub, api") {
		t.Errorf("create_spec with unknown repo must fail listing known ones, got %.200s (err=%v)", text, isErr)
	}
	if matches, _ := filepath.Glob(filepath.Join(repo, ".truthboard", "specs", "*bad-child*")); len(matches) != 0 {
		t.Errorf("failed create must not leave an orphan file: %v", matches)
	}

	// next_spec hands out the api half; the hub half waits on it.
	if text, isErr := toolText(t, responses[3]); isErr || !strings.Contains(text, "Next up: tb-mcp1") || strings.Contains(text, child.ID+" —") {
		t.Errorf("next_spec must hand out the unblocked half, got %.200s (err=%v)", text, isErr)
	}
}
