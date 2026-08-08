package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A spoke can be perfectly readable — proof flows from it — while the agents
// working in it have no board at all. The audit reports that as drift, and
// still never writes anything.
func TestWorkspaceUnwiredSpokeIsDrift(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init spoke", now.AddDate(0, 0, -10))

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Workspace) != 1 || res.Workspace[0].Err != "" {
		t.Fatalf("the spoke is readable — wiring is a separate question: %+v", res.Workspace)
	}
	if len(res.Drift.UnwiredRepos) != 1 {
		t.Fatalf("want one unwired spoke, got %v", res.Drift.UnwiredRepos)
	}
	for _, want := range []string{"api", "no MCP registration", "truthboard init --workspace"} {
		if !strings.Contains(res.Drift.UnwiredRepos[0], want) {
			t.Errorf("finding missing %q: %s", want, res.Drift.UnwiredRepos[0])
		}
	}
	// Read-only: detecting a gap must not close it.
	if _, err := os.Stat(filepath.Join(spoke.dir, ".mcp.json")); !os.IsNotExist(err) {
		t.Error("the audit wired a spoke — it must only ever report")
	}
}

// A single repo has no spokes, so this finding can never fire there.
func TestSingleRepoHasNoWiringFinding(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: init", now.AddDate(0, 0, -3))

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.UnwiredRepos) != 0 {
		t.Fatalf("a workspace of one must never report unwired spokes, got %v", res.Drift.UnwiredRepos)
	}
}
