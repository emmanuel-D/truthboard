package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mcpFixture builds a repository carrying one spec, so a tool call reading
// the wrong directory is visible as a missing story rather than as a
// subtly different board.
func mcpFixture(t *testing.T, id, title string) string {
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
	md := "---\nid: " + id + "\ntitle: " + title + "\npriority: 1\n---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(specDir, id+"-test.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const listSpecs = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_specs","arguments":{}}}`

// TestMcpServesTheRepoArgumentNotTheWorkingDirectory is the bug: the path
// on the command line was accepted and then ignored, so an agent launched
// from a workspace parent got a server reading the parent, and the only
// workaround was baking an absolute path into a committed .mcp.json.
func TestMcpServesTheRepoArgumentNotTheWorkingDirectory(t *testing.T) {
	hub := mcpFixture(t, "tb-hub1", "The story only the hub has")
	elsewhere := mcpFixture(t, "tb-else", "The story in the wrong repo")
	t.Chdir(elsewhere) // the directory an MCP client happened to spawn us in

	var out bytes.Buffer
	if code := runMcp([]string{hub}, strings.NewReader(listSpecs+"\n"), &out); code != 0 {
		t.Fatalf("runMcp = %d, want 0: %s", code, out.String())
	}
	body := out.String()
	if !strings.Contains(body, "tb-hub1") {
		t.Errorf("the server did not read the repository it was pointed at:\n%s", body)
	}
	if strings.Contains(body, "tb-else") {
		t.Errorf("the server read its working directory instead of the argument:\n%s", body)
	}
}

// With no argument the behaviour is what every MCP client already relies
// on: serve the directory the process was started in.
func TestMcpWithoutAnArgumentServesTheWorkingDirectory(t *testing.T) {
	here := mcpFixture(t, "tb-cwd1", "The story in the working directory")
	t.Chdir(here)

	var out bytes.Buffer
	if code := runMcp(nil, strings.NewReader(listSpecs+"\n"), &out); code != 0 {
		t.Fatalf("runMcp = %d, want 0: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "tb-cwd1") {
		t.Errorf("the default repo is no longer the working directory:\n%s", out.String())
	}
}

// A directory that is not a repository must fail at startup. Starting and
// then failing per tool call is the same information delivered as noise —
// and it arrives after the handshake has told the agent everything is fine.
func TestMcpRefusesANonRepositoryBeforeTheHandshake(t *testing.T) {
	t.Chdir(mcpFixture(t, "tb-cwd2", "not the one asked for"))
	notARepo := t.TempDir()

	var out bytes.Buffer
	code := runMcp([]string{notARepo}, strings.NewReader(listSpecs+"\n"), &out)
	if code != 1 {
		t.Fatalf("runMcp on a plain directory = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("a refused server must not answer the handshake, got:\n%s", out.String())
	}
}

// The generated .mcp.json is committed and shared, so it must keep working
// on a teammate's machine: the repo argument exists to be optional, never
// to put this machine's paths into a portable file.
func TestGeneratedMcpConfigStaysPortable(t *testing.T) {
	dir := mcpFixture(t, "tb-cfg1", "portable config")
	if code := runInit([]string{"--agents", dir}); code != 0 {
		t.Fatalf("truthboard init --agents = %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	tb, ok := doc.MCPServers["truthboard"]
	if !ok {
		t.Fatalf("no truthboard server in .mcp.json:\n%s", raw)
	}
	for _, arg := range append([]string{tb.Command}, tb.Args...) {
		if filepath.IsAbs(arg) {
			t.Errorf("generated .mcp.json carries an absolute path %q — it must survive being committed", arg)
		}
	}
}
