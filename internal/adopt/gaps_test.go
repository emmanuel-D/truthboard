package adopt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGapsSaysNothingAboutAWiredWorkspace(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Agents(hub, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Spokes(hub, true); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 0 {
		t.Errorf("a freshly wired workspace must be clean, got %v", gaps)
	}
}

func TestGapsNamesTheSpokeAndTheFix(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Spokes(hub, false); err != nil {
		t.Fatal(err)
	}
	// web is cloned fresh: watched for proof, wired for nothing.
	api := filepath.Join(hub, "..", "web")
	for _, f := range []string{".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md"} {
		if err := os.Remove(filepath.Join(api, filepath.FromSlash(f))); err != nil {
			t.Fatal(err)
		}
	}

	gaps := Gaps(hub)
	if len(gaps) != 1 {
		t.Fatalf("want exactly the unwired spoke, got %v", gaps)
	}
	for _, want := range []string{"web", "../web", "no MCP registration", "no working agreement", "truthboard init --workspace"} {
		if !strings.Contains(gaps[0], want) {
			t.Errorf("finding missing %q: %s", want, gaps[0])
		}
	}
}

func TestGapsCatchesAnMCPServerPointedElsewhere(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Spokes(hub, false); err != nil {
		t.Fatal(err)
	}
	api := filepath.Join(hub, "..", "api")

	// A hand-edited registration that serves the spoke itself: the server
	// starts, every tool call answers from a repo with no specs in it.
	if err := os.WriteFile(filepath.Join(api, ".mcp.json"),
		[]byte(`{"mcpServers":{"truthboard":{"command":"truthboard","args":["mcp"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 1 || !strings.Contains(gaps[0], "must serve the hub") {
		t.Errorf("a spoke serving itself must be reported, got %v", gaps)
	}

	if err := os.WriteFile(filepath.Join(api, ".mcp.json"),
		[]byte(`{"mcpServers":{"truthboard":{"command":"truthboard","args":["mcp","../elsewhere"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 1 || !strings.Contains(gaps[0], "points at ../elsewhere") {
		t.Errorf("a server pointed at the wrong repo must be reported, got %v", gaps)
	}
}

func TestGapsIgnoresSpokesItCannotSee(t *testing.T) {
	// web has no checkout at all — that is the audit's unreadable-spoke
	// report, and saying it twice trains people to skim both.
	hub := hubWithSpokes(t, `
repos:
  api:
    remote: git@example.com:acme/api.git
    path: ../api
  web:
    remote: git@example.com:acme/web.git
`, "api")
	if _, err := Spokes(hub, false); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 0 {
		t.Errorf("a spoke with no local copy is not a wiring gap, got %v", gaps)
	}
}

func TestGapsTreatsTheNudgeAsOptional(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Agents(hub, true); err != nil { // hub takes the nudge
		t.Fatal(err)
	}
	if _, err := Spokes(hub, false); err != nil { // spokes do not
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 0 {
		t.Errorf("a missing opt-in nudge is not a finding on its own, got %v", gaps)
	}

	// Once a spoke is reported for a real gap, the nudge the hub has and it
	// does not is part of what the re-run will fix — worth naming there.
	if err := os.Remove(filepath.Join(hub, "..", "api", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	gaps := Gaps(hub)
	if len(gaps) != 1 || !strings.Contains(gaps[0], "no commit-msg trailer nudge") {
		t.Errorf("want the nudge named alongside a real gap, got %v", gaps)
	}
}

func TestGapsIgnoresASingleRepo(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(repo); gaps != nil {
		t.Errorf("a workspace of one has no spokes to report, got %v", gaps)
	}
}
