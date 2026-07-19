package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeWorkspace(t *testing.T, hub, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(hub, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hub, ".truthboard", "workspace.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestWorkspaceSpokeLandingFlipsSpecDone is the core multi-repo promise:
// intent lives in the hub, the trailer lands in a spoke, and the board
// derives done with evidence naming the spoke.
func TestWorkspaceSpokeLandingFlipsSpecDone(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init spoke", now.AddDate(0, 0, -10))
	spoke.git("checkout", "-b", "feature/tb-aaaa-api-half")
	spoke.commit("feat: api half\n\nSpec: tb-aaaa", now.AddDate(0, 0, -2))
	spoke.git("checkout", "main")
	spoke.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-aaaa-api-half'", "feature/tb-aaaa-api-half")
	spoke.git("branch", "-D", "feature/tb-aaaa-api-half")

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpec(t, hub.dir, "tb-aaaa", "Cross-repo story", "")
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Workspace) != 1 || res.Workspace[0].Err != "" {
		t.Fatalf("expected one healthy spoke, got %+v", res.Workspace)
	}
	s := specByID(t, res, "tb-aaaa")
	if s.Status != Done {
		t.Fatalf("spec should be done via the spoke landing, got %s (%s)", s.Status, s.Evidence)
	}
	if s.LandedRepo != "api" {
		t.Fatalf("landed repo should be api, got %q", s.LandedRepo)
	}
	if !strings.Contains(s.Evidence, "api:main") {
		t.Fatalf("evidence should name the spoke, got %q", s.Evidence)
	}
}

// TestWorkspaceActiveSpokeWorkOutranksHubLanding: landing part of the work
// in the hub while a spoke branch still moves means not finished.
func TestWorkspaceActiveSpokeWorkOutranksHubLanding(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init spoke", now.AddDate(0, 0, -10))
	spoke.git("checkout", "-b", "feature/tb-bbbb-web-half")
	spoke.commit("feat: web half, in flight", now.AddDate(0, 0, -1))
	spoke.git("checkout", "main")

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	hub.git("checkout", "-b", "feature/tb-bbbb-hub-half")
	hub.commit("feat: hub half\n\nSpec: tb-bbbb", now.AddDate(0, 0, -2))
	hub.git("checkout", "main")
	hub.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-bbbb-hub-half'", "feature/tb-bbbb-hub-half")
	hub.git("branch", "-D", "feature/tb-bbbb-hub-half")
	writeSpec(t, hub.dir, "tb-bbbb", "Cross-repo story", "")
	writeWorkspace(t, hub.dir, "repos:\n  web:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := specByID(t, res, "tb-bbbb")
	if s.Status != InProgress {
		t.Fatalf("active spoke work must outrank the hub landing, got %s (%s)", s.Status, s.Evidence)
	}
	if !strings.Contains(s.Evidence, "web:feature/tb-bbbb-web-half") {
		t.Fatalf("evidence should carry the repo-prefixed branch, got %q", s.Evidence)
	}

	var unit *Unit
	for i := range res.Units {
		if res.Units[i].Repo == "web" {
			unit = &res.Units[i]
		}
	}
	if unit == nil {
		t.Fatalf("spoke unit missing from %+v", res.Units)
	}
	if unit.SpecID != "tb-bbbb" || unit.Label() != "web:feature/tb-bbbb-web-half" {
		t.Fatalf("spoke unit not linked/labeled: %+v", unit)
	}
}

// TestWorkspaceUnreachableSpokeIsLoud: a spoke the audit cannot read is a
// finding on the result, never a silent omission — and the hub still derives.
func TestWorkspaceUnreachableSpokeIsLoud(t *testing.T) {
	now := time.Now()
	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpec(t, hub.dir, "tb-cccc", "Some story", "")
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    remote: git@example.invalid:acme/api.git\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Workspace) != 1 || res.Workspace[0].Err == "" {
		t.Fatalf("unreachable spoke must carry an error, got %+v", res.Workspace)
	}
	if s := specByID(t, res, "tb-cccc"); s.Status != Planned {
		t.Fatalf("hub derivation should survive a dead spoke, got %s", s.Status)
	}
}

// TestWorkspaceScopeCreepRepoPrefix: an api:-prefixed path pattern scopes
// creep detection to that spoke; hub patterns never judge spoke branches.
func TestWorkspaceScopeCreepRepoPrefix(t *testing.T) {
	got := pathsFor("api", []string{"internal/**", "api:src/auth/**", "web:ui/**"})
	if len(got) != 1 || got[0] != "src/auth/**" {
		t.Fatalf("pathsFor(api) = %v", got)
	}
	got = pathsFor("", []string{"internal/**", "api:src/auth/**"})
	if len(got) != 1 || got[0] != "internal/**" {
		t.Fatalf("pathsFor(hub) = %v", got)
	}
	// A colon inside a glob or an URL-ish token is not a repo prefix.
	if _, _, ok := splitRepoPrefix("https://example.com/x"); ok {
		t.Fatal("https:// must not parse as a repo prefix")
	}
}
