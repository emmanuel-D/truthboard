package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The audit's unwired-spoke finding names `truthboard init --workspace` as
// the fix, so that command has to work on a hub with nothing new to declare
// — which is exactly the case where a spoke was checked out after setup.
func TestInitWorkspaceRerunWiresASpokeCheckedOutLater(t *testing.T) {
	root := t.TempDir()
	hub, api := filepath.Join(root, "hub"), filepath.Join(root, "api")
	for _, dir := range []string{hub, api} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
	}
	if code := runInit([]string{"--workspace", "api=git@example.com:acme/api.git", "--path", "api=../api", hub}); code != 0 {
		t.Fatalf("first run = %d", code)
	}
	// A teammate's fresh clone: declared, watched, wired for nothing.
	for _, f := range []string{".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md"} {
		if err := os.RemoveAll(filepath.Join(api, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}

	if code := runInit([]string{"--workspace", hub}); code != 0 {
		t.Fatalf("re-run with nothing new to declare = %d, want 0", code)
	}
	for _, f := range []string{".mcp.json", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(api, f)); err != nil {
			t.Errorf("re-run did not restore %s: %v", f, err)
		}
	}
}

// A first run with nothing to declare is still an error: there is no
// workspace to re-apply, and a hub with an empty manifest watches nothing.
func TestInitWorkspaceNeedsSpokesOnAFirstRun(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if code := runInit([]string{"--workspace", dir}); code != 2 {
		t.Fatalf("first run with no spokes = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, ".truthboard", "workspace.yml")); !os.IsNotExist(err) {
		t.Error("a refused run must not leave a manifest behind")
	}
}
