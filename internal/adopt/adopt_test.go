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
	for _, want := range []string{"registered the truthboard MCP server", "working agreement written", "agreement import written", "installed"} {
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

	// Claude Code loads CLAUDE.md but not AGENTS.md, so adoption must
	// create it and import the agreement.
	claude, err := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be created on a fresh repo: %v", err)
	}
	if !strings.Contains(string(claude), "@AGENTS.md") {
		t.Errorf("CLAUDE.md missing the @AGENTS.md import:\n%s", claude)
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

func TestAgentsUpgradesStalePointerBlock(t *testing.T) {
	repo := gitRepo(t)
	stale := "# My project\n\n" + beginMark + "\n## Task tracking\n\nOld pointer without the import.\n" + endMark + "\n"
	os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(stale), 0o644)

	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if !strings.Contains(string(claude), "# My project") {
		t.Errorf("CLAUDE.md lost the owner's content:\n%s", claude)
	}
	if !strings.Contains(string(claude), "@AGENTS.md") || strings.Contains(string(claude), "Old pointer") {
		t.Errorf("stale block should be replaced with the import:\n%s", claude)
	}
	if n := strings.Count(string(claude), beginMark); n != 1 {
		t.Errorf("CLAUDE.md has %d truthboard blocks, want exactly 1", n)
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

func TestSpawnWarningWhenBinaryOnlyInShellProfile(t *testing.T) {
	sysDir := t.TempDir() // empty: plays the role of /usr/local/bin
	goBin := filepath.Join(t.TempDir(), "go", "bin")
	os.MkdirAll(goBin, 0o755)
	exe := filepath.Join(goBin, "truthboard")
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)

	lines := spawnWarning(exe, []string{sysDir})
	if len(lines) == 0 {
		t.Fatal("expected a warning when no system dir has truthboard")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{exe, "ln -s", sysDir, "silently"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning missing %q:\n%s", want, joined)
		}
	}
}

func TestSpawnWarningQuietWhenSystemDirHasBinary(t *testing.T) {
	sysDir := t.TempDir()
	os.WriteFile(filepath.Join(sysDir, "truthboard"), []byte("#!/bin/sh\n"), 0o755)

	if lines := spawnWarning("/home/u/go/bin/truthboard", []string{sysDir}); lines != nil {
		t.Errorf("expected no warning, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSpawnWarningIgnoresNonExecutable(t *testing.T) {
	sysDir := t.TempDir()
	os.WriteFile(filepath.Join(sysDir, "truthboard"), []byte("not a binary"), 0o644)

	if lines := spawnWarning("/home/u/go/bin/truthboard", []string{sysDir}); len(lines) == 0 {
		t.Error("a non-executable file must not count as resolvable")
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
