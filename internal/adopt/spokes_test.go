package adopt

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// hubWithSpokes builds a workspace on disk — a hub and a git checkout per
// named spoke, side by side — and returns the hub path. manifest is written
// verbatim so a test can declare a spoke the layout does not have.
func hubWithSpokes(t *testing.T, manifest string, checkouts ...string) string {
	t.Helper()
	root := t.TempDir()
	hub := filepath.Join(root, "hub")
	if err := os.MkdirAll(filepath.Join(hub, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", hub, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init hub: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(hub, ".truthboard", "workspace.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range checkouts {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", name, err, out)
		}
	}
	return hub
}

func mcpArgs(t *testing.T, path, key string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	server, ok := doc[key]["truthboard"]
	if !ok {
		t.Fatalf("%s: no truthboard server registered", path)
	}
	if server.Command != "truthboard" {
		t.Errorf("%s: command = %q, want truthboard", path, server.Command)
	}
	return server.Args
}

const twoSpokes = `
repos:
  api:
    remote: git@example.com:acme/api.git
    path: ../api
  web:
    remote: git@example.com:acme/web.git
    path: ../web
`

func TestSpokesWiresEveryLocalCheckout(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	log, err := Spokes(hub, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")

	for _, name := range []string{"api", "web"} {
		spoke := filepath.Join(hub, "..", name)
		// The MCP server must serve the hub, not the spoke: a bare "mcp"
		// here would serve a repository with no specs in it.
		for _, f := range []struct{ path, key string }{
			{filepath.Join(spoke, ".mcp.json"), "mcpServers"},
			{filepath.Join(spoke, ".vscode", "mcp.json"), "servers"},
		} {
			args := mcpArgs(t, f.path, f.key)
			if len(args) != 2 || args[0] != "mcp" || args[1] != "../hub" {
				t.Errorf("%s: args = %v, want [mcp ../hub]", f.path, args)
			}
		}

		agents, err := os.ReadFile(filepath.Join(spoke, "AGENTS.md"))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Spec: tb-1234", "workspace spoke", "`../hub`", "`api`, `web`"} {
			if !strings.Contains(string(agents), want) {
				t.Errorf("%s AGENTS.md missing %q:\n%s", name, want, agents)
			}
		}
		// The spoke has no specs of its own, so it must not be told to edit
		// files that do not exist there.
		if strings.Contains(string(agents), ".truthboard/specs/*.md") {
			t.Errorf("%s AGENTS.md points at spec files that live in the hub", name)
		}
		if _, err := os.Stat(filepath.Join(spoke, ".truthboard")); !os.IsNotExist(err) {
			t.Errorf("%s: a spoke must not get a .truthboard directory (that makes it a second hub)", name)
		}

		claude, err := os.ReadFile(filepath.Join(spoke, "CLAUDE.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(claude), "@AGENTS.md") {
			t.Errorf("%s CLAUDE.md does not import the agreement", name)
		}
		hook, err := os.ReadFile(filepath.Join(spoke, ".git", "hooks", "commit-msg"))
		if err != nil {
			t.Fatalf("%s: no commit-msg hook: %v", name, err)
		}
		if !strings.Contains(string(hook), "Spec: tb-") {
			t.Errorf("%s: hook is not the trailer nudge", name)
		}
		if !strings.Contains(joined, "spoke "+name) {
			t.Errorf("log does not name spoke %s:\n%s", name, joined)
		}
	}
}

func TestSpokesReportsUnwirableSpokesByNameAndWiresTheRest(t *testing.T) {
	// api is checked out; web is remote-only, and cloud has a path that does
	// not exist. Neither may stop api from being wired.
	hub := hubWithSpokes(t, `
repos:
  api:
    remote: git@example.com:acme/api.git
    path: ../api
  cloud:
    remote: git@example.com:acme/cloud.git
    path: ../cloud
  web:
    remote: git@example.com:acme/web.git
`, "api")
	log, err := Spokes(hub, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")

	if _, err := os.Stat(filepath.Join(hub, "..", "api", ".mcp.json")); err != nil {
		t.Errorf("api was not wired despite being checked out: %v", err)
	}
	for _, want := range []string{"spoke web: not wired", "spoke cloud: not wired", "re-run this command"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log missing %q:\n%s", want, joined)
		}
	}
	// Adoption never clones: nothing may appear where the missing spokes live.
	for _, name := range []string{"web", "cloud"} {
		if _, err := os.Stat(filepath.Join(hub, "..", name)); !os.IsNotExist(err) {
			t.Errorf("%s: adoption created a directory for a spoke with no checkout", name)
		}
	}
}

func TestSpokesIsIdempotentAndPreservesWhatItFinds(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	api := filepath.Join(hub, "..", "api")
	if err := os.WriteFile(filepath.Join(api, ".mcp.json"),
		[]byte(`{"mcpServers":{"other":{"command":"other"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(api, "AGENTS.md"),
		[]byte("# House rules\n\nRun the linter.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Spokes(hub, true); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(filepath.Join(api, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"other"`) {
		t.Errorf("wiring dropped someone else's MCP server:\n%s", raw)
	}
	agents, err := os.ReadFile(filepath.Join(api, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "Run the linter.") {
		t.Errorf("wiring dropped existing AGENTS.md prose:\n%s", agents)
	}

	before := snapshot(t, api)
	log, err := Spokes(hub, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot(t, api); got != before {
		t.Errorf("second run changed the spoke:\nbefore:\n%s\nafter:\n%s", before, got)
	}
	for _, want := range []string{"already registered", "already up to date"} {
		if !strings.Contains(strings.Join(log, "\n"), want) {
			t.Errorf("second run does not report a no-op (%q):\n%s", want, strings.Join(log, "\n"))
		}
	}
}

// snapshot renders every wired file's content so a re-run can be compared
// byte for byte.
func snapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	for _, name := range []string{".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md", ".git/hooks/commit-msg"} {
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		b.WriteString("=== " + name + "\n" + string(raw))
	}
	return b.String()
}

func TestSpokesRefusesACheckoutOfAnotherRepository(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	api := filepath.Join(hub, "..", "api")
	if out, err := exec.Command("git", "-C", api, "remote", "add", "origin",
		"git@example.com:acme/not-api.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v\n%s", err, out)
	}
	log, err := Spokes(hub, false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "spoke api: not wired") || !strings.Contains(joined, "not-api") {
		t.Errorf("a path holding a different repository must not be wired:\n%s", joined)
	}
	if _, err := os.Stat(filepath.Join(api, "AGENTS.md")); !os.IsNotExist(err) {
		t.Error("wrote an agreement into a checkout of a different repository")
	}
}

func TestSpokesIgnoresASingleRepo(t *testing.T) {
	repo := gitRepo(t)
	log, err := Spokes(repo, true)
	if err != nil || log != nil {
		t.Fatalf("no manifest means nothing to wire, got %v, %v", log, err)
	}
}

func TestHubRelativeToIsPortable(t *testing.T) {
	root := t.TempDir()
	got, err := hubRelativeTo(filepath.Join(root, "api"), filepath.Join(root, "hub"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "../hub" {
		t.Errorf("hubRelativeTo = %q, want ../hub", got)
	}
	if _, err := hubRelativeTo(root, root); err == nil {
		t.Error("a spoke path pointing at the hub itself must not be wired")
	}
}
