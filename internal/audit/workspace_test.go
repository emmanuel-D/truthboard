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

func writeSpecRepos(t *testing.T, repo, id, title string, repos []string) {
	t.Helper()
	dir := filepath.Join(repo, ".truthboard", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\nrepos:\n"
	for _, r := range repos {
		content += "    - " + r + "\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDeclaredReposPartialLandingIsNeverDone: repos: [hub, api] with only
// the hub landed derives in-progress with per-repo chips — git can now see
// that the api half was promised and is missing.
func TestDeclaredReposPartialLandingIsNeverDone(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	hub.git("checkout", "-b", "feature/tb-dddd-hub-half")
	hub.commit("feat: hub half\n\nSpec: tb-dddd", now.AddDate(0, 0, -2))
	hub.git("checkout", "main")
	hub.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-dddd-hub-half'", "feature/tb-dddd-hub-half")
	hub.git("branch", "-D", "feature/tb-dddd-hub-half")
	writeSpecRepos(t, hub.dir, "tb-dddd", "Cross-repo story", []string{"hub", "api"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := specByID(t, res, "tb-dddd")
	if s.Status != InProgress {
		t.Fatalf("partial landing must derive in-progress, got %s (%s)", s.Status, s.Evidence)
	}
	if !strings.Contains(s.Evidence, "hub ✓ landed") || !strings.Contains(s.Evidence, "api — no branch yet") {
		t.Fatalf("evidence must chip per repo, got %q", s.Evidence)
	}
	if len(s.PerRepo) != 2 || s.PerRepo[0].State != "landed" || s.PerRepo[1].State != "missing" {
		t.Fatalf("per_repo wrong: %+v", s.PerRepo)
	}
	if s.LandedRepo != "" || s.Landed == "" {
		t.Fatalf("hub landing must fill Landed with LandedRepo empty, got %q/%q", s.Landed, s.LandedRepo)
	}

	// Landing the api half flips it done.
	spoke.git("checkout", "-b", "feature/tb-dddd-api-half")
	spoke.commit("feat: api half\n\nSpec: tb-dddd", now.AddDate(0, 0, -1))
	spoke.git("checkout", "main")
	spoke.gitAt(now.AddDate(0, 0, -1), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-dddd-api-half'", "feature/tb-dddd-api-half")
	spoke.git("branch", "-D", "feature/tb-dddd-api-half")

	res, err = Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s = specByID(t, res, "tb-dddd")
	if s.Status != Done {
		t.Fatalf("all declared repos landed must derive done, got %s (%s)", s.Status, s.Evidence)
	}
	if !strings.Contains(s.Evidence, "hub ✓ landed") || !strings.Contains(s.Evidence, "api ✓ landed") {
		t.Fatalf("done evidence must show both chips, got %q", s.Evidence)
	}
}

// TestDeclaredReposStalledBranchShows: the missing repo's branch state
// drives the spec status when nothing is active.
func TestDeclaredReposStalledBranchShows(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -40))
	spoke.git("checkout", "-b", "feature/tb-eeee-api")
	spoke.commit("feat: went quiet", now.AddDate(0, 0, -30))
	spoke.git("checkout", "main")

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpecRepos(t, hub.dir, "tb-eeee", "Story", []string{"api"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := specByID(t, res, "tb-eeee")
	if s.Status != Stalled {
		t.Fatalf("stalled branch in the declared repo must derive stalled, got %s (%s)", s.Status, s.Evidence)
	}
	if !strings.Contains(s.Evidence, "api — stalled (feature/tb-eeee-api)") {
		t.Fatalf("evidence must name the stalled branch, got %q", s.Evidence)
	}
}

// TestDeclaredReposUnknownRepoIsLoud: a repos: entry the workspace doesn't
// declare is a drift finding and keeps the spec from deriving done even
// when everything else landed.
func TestDeclaredReposUnknownRepoIsLoud(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	hub.git("checkout", "-b", "feature/tb-ffff-hub")
	hub.commit("feat: hub work\n\nSpec: tb-ffff", now.AddDate(0, 0, -2))
	hub.git("checkout", "main")
	hub.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-ffff-hub'", "feature/tb-ffff-hub")
	hub.git("branch", "-D", "feature/tb-ffff-hub")
	writeSpecRepos(t, hub.dir, "tb-ffff", "Story", []string{"hub", "mobile"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := specByID(t, res, "tb-ffff")
	if s.Status == Done {
		t.Fatalf("a spec requiring an unknown repo must never derive done, got %s", s.Status)
	}
	if !strings.Contains(s.Evidence, "mobile ✗ not in workspace") {
		t.Fatalf("evidence must flag the unknown repo, got %q", s.Evidence)
	}
	if len(res.Drift.UnknownRepos) != 1 || !strings.Contains(res.Drift.UnknownRepos[0], "tb-ffff") {
		t.Fatalf("unknown repo must be a drift finding, got %v", res.Drift.UnknownRepos)
	}
}

// TestDeclaredReposRevertInSpokeRegresses: a revert in any declared repo
// flips a done spec to regressed, evidence naming the repo.
func TestDeclaredReposRevertInSpokeRegresses(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))
	spoke.git("checkout", "-b", "feature/tb-abab-api")
	spoke.commit("feat: api work\n\nSpec: tb-abab", now.AddDate(0, 0, -3))
	spoke.git("checkout", "main")
	spoke.gitAt(now.AddDate(0, 0, -3), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-abab-api'", "feature/tb-abab-api")
	spoke.git("branch", "-D", "feature/tb-abab-api")

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpecRepos(t, hub.dir, "tb-abab", "Story", []string{"api"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if s := specByID(t, res, "tb-abab"); s.Status != Done {
		t.Fatalf("precondition: spec should be done, got %s (%s)", s.Status, s.Evidence)
	}

	sha := spoke.git("log", "--grep", "Spec: tb-abab", "--format=%H", "-n", "1", "main")
	spoke.gitAt(now, "revert", "--no-edit", "-m", "1", sha)

	res, err = Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := specByID(t, res, "tb-abab")
	if s.Status != Regressed {
		t.Fatalf("revert in the declared spoke must regress, got %s (%s)", s.Status, s.Evidence)
	}
	if !strings.HasPrefix(s.Evidence, "api: ") {
		t.Fatalf("regression evidence must name the repo, got %q", s.Evidence)
	}
}

// TestBriefSurfacesWorkspace: in a workspace, the brief carries the repo
// list plus the split-or-declare instruction (undeclared) or the per-repo
// landing contract (declared).
func TestBriefSurfacesWorkspace(t *testing.T) {
	now := time.Now()
	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpec(t, hub.dir, "tb-1a1a", "Undeclared story", "")
	writeSpecRepos(t, hub.dir, "tb-2b2b", "Declared story", []string{"hub", "api"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	brief, err := Brief(hub.dir, "tb-1a1a")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Workspace: this hub gathers proof from api", "split before coding", "declare repos:"} {
		if !strings.Contains(brief, want) {
			t.Errorf("undeclared brief missing %q in:\n%s", want, brief)
		}
	}

	brief, err = Brief(hub.dir, "tb-2b2b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief, "declares repos: hub, api") || !strings.Contains(brief, "every one") {
		t.Errorf("declared brief missing the per-repo landing contract:\n%s", brief)
	}
	if strings.Contains(brief, "split before coding") {
		t.Errorf("declared brief must not also tell the agent to split:\n%s", brief)
	}

	// No workspace: no workspace talk at all.
	plain := newFixture(t)
	plain.commit("chore: init", now.AddDate(0, 0, -10))
	writeSpec(t, plain.dir, "tb-3c3c", "Plain story", "")
	brief, err = Brief(plain.dir, "tb-3c3c")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(brief, "Workspace:") {
		t.Errorf("plain repo brief must not mention workspaces:\n%s", brief)
	}
}

// TestNextRespectsCrossRepoNeeds: a story needing an api-half that has not
// landed in the api spoke is never handed out; the moment the spoke landing
// happens, it becomes the next story.
func TestNextRespectsCrossRepoNeeds(t *testing.T) {
	now := time.Now()
	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpecRepos(t, hub.dir, "tb-aaa1", "API half", []string{"api"})
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")
	dir := filepath.Join(hub.dir, ".truthboard", "specs")
	web := "---\nid: tb-bbb2\ntitle: Web half\nneeds:\n    - tb-aaa1\n---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, "tb-bbb2-test.md"), []byte(web), 0o644); err != nil {
		t.Fatal(err)
	}

	next, _, waiting, err := Next(hub.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "tb-aaa1" {
		t.Fatalf("next should be the api half, got %+v", next)
	}
	if len(waiting) != 1 || waiting[0].ID != "tb-bbb2" {
		t.Fatalf("web half should be waiting on the api landing, got %+v", waiting)
	}

	spoke.git("checkout", "-b", "feature/tb-aaa1-api")
	spoke.commit("feat: api half\n\nSpec: tb-aaa1", now.AddDate(0, 0, -1))
	spoke.git("checkout", "main")
	spoke.gitAt(now.AddDate(0, 0, -1), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-aaa1-api'", "feature/tb-aaa1-api")
	spoke.git("branch", "-D", "feature/tb-aaa1-api")

	next, _, waiting, err = Next(hub.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "tb-bbb2" {
		t.Fatalf("after the spoke landing the web half must be next, got %+v (waiting %+v)", next, waiting)
	}
}
