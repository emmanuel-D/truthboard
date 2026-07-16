package adopt

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestAgentsWiresFreshRepo(t *testing.T) {
	repo := gitRepo(t)
	log, err := Agents(repo, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	for _, want := range []string{"registered the truthboard MCP server", "working agreement written", "not present, skipped", "installed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log missing %q:\n%s", want, joined)
		}
	}

	var mcp struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &mcp); err != nil {
		t.Fatal(err)
	}
	if s, ok := mcp.Servers["truthboard"]; !ok || s.Command != "truthboard" || len(s.Args) != 1 || s.Args[0] != "mcp" {
		t.Errorf(".mcp.json = %+v, want truthboard mcp server", mcp.Servers)
	}

	agents, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Spec: tb-1234", "get_brief", "never typed"} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md missing %q", want)
		}
	}

	hook := filepath.Join(repo, ".git", "hooks", "commit-msg")
	info, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("commit-msg hook is not executable")
	}
}

func TestAgentsIsIdempotent(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md"))

	log, err := Agents(repo, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	for _, want := range []string{"already registered", "already up to date", "already installed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("second run should be a no-op, log:\n%s", joined)
		}
	}
	second, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if string(first) != string(second) {
		t.Error("AGENTS.md changed on second run")
	}
	if n := strings.Count(string(second), beginMark); n != 1 {
		t.Errorf("AGENTS.md has %d truthboard blocks, want exactly 1", n)
	}
}

func TestAgentsPreservesOthersFiles(t *testing.T) {
	repo := gitRepo(t)
	// Pre-existing .mcp.json with another server, CLAUDE.md with content,
	// and a commit-msg hook that blocks — all must survive.
	os.WriteFile(filepath.Join(repo, ".mcp.json"),
		[]byte(`{"mcpServers":{"other":{"command":"other-tool"}}}`), 0o644)
	os.WriteFile(filepath.Join(repo, "CLAUDE.md"),
		[]byte("# My project\n\nBuild with make.\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"),
		[]byte("#!/bin/sh\ngrep -q JIRA \"$1\" || exit 1\n"), 0o755)

	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if !strings.Contains(string(raw), "other-tool") || !strings.Contains(string(raw), "truthboard") {
		t.Errorf(".mcp.json lost a server:\n%s", raw)
	}
	claude, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if !strings.Contains(string(claude), "Build with make.") || !strings.Contains(string(claude), "Truthboard") {
		t.Errorf("CLAUDE.md content wrong:\n%s", claude)
	}
	hook, _ := os.ReadFile(filepath.Join(repo, ".git", "hooks", "commit-msg"))
	if !strings.Contains(string(hook), "JIRA") || !strings.Contains(string(hook), hookMark) {
		t.Errorf("existing hook logic lost:\n%s", hook)
	}
	// The nudge must sit before the blocking logic and never add an exit.
	if strings.Index(string(hook), hookMark) > strings.Index(string(hook), "JIRA") {
		t.Error("nudge should be inserted before the existing hook logic")
	}
	if strings.Contains(strings.TrimPrefix(string(hook), hookScript), "exit 0") &&
		strings.Count(string(hook), "exit 0") > strings.Count("#!/bin/sh\ngrep -q JIRA \"$1\" || exit 1\n", "exit 0") {
		t.Error("insertion must not add exit statements to someone else's hook")
	}
}

func TestHookWarnsButNeverBlocks(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(repo, ".git", "hooks", "commit-msg")

	run := func(msg string) (string, error) {
		msgFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
		os.WriteFile(msgFile, []byte(msg), 0o644)
		out, err := exec.Command("/bin/sh", hook, msgFile).CombinedOutput()
		return string(out), err
	}

	out, err := run("feat: no trailer here")
	if err != nil {
		t.Errorf("hook must never block, got error %v", err)
	}
	if !strings.Contains(out, "shadow work") {
		t.Errorf("expected a warning for missing trailer, got %q", out)
	}

	for _, msg := range []string{"feat: linked\n\nSpec: tb-1234", "Merge branch 'x'", "Revert \"feat: y\""} {
		out, err := run(msg)
		if err != nil || out != "" {
			t.Errorf("message %q should pass silently, got out=%q err=%v", msg, out, err)
		}
	}
}
