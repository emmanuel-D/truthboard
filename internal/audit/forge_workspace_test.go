package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/forge"
)

// spokeWithForgeFixture: a hub whose workspace declares one spoke "api".
// The spoke carries a merged "fixes #1" commit, a live spec-linked branch,
// and a live unticketed branch — everything per-spoke enrichment must see.
func spokeWithForgeFixture(t *testing.T, now time.Time) (hub, spoke *fixture) {
	spoke = newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))

	spoke.git("checkout", "-b", "feature/fixer")
	spoke.commit("feat: repair sync, fixes #1", now.AddDate(0, 0, -3))
	spoke.git("checkout", "main")
	spoke.gitAt(now.AddDate(0, 0, -3), "merge", "--no-ff",
		"-m", "Merge branch 'feature/fixer'", "feature/fixer")
	spoke.git("branch", "-D", "feature/fixer")

	spoke.git("checkout", "-b", "feature/tb-spk1-api")
	spoke.commit("feat: api work\n\nSpec: tb-spk1", now.AddDate(0, 0, -1))

	spoke.git("checkout", "-b", "feature/mystery", "main")
	spoke.commit("feat: mystery work", now.AddDate(0, 0, -1))
	spoke.git("checkout", "main")

	hub = newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	writeSpec(t, hub.dir, "tb-spk1", "Spoke story", "")
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")
	return hub, spoke
}

// TestSpokeForgeEnrichment is the core per-spoke promise: the spoke's own
// forge upgrades its branches and audits its tracker, with every finding
// carrying the repo name — while the hub, forge-less, stays git-only.
func TestSpokeForgeEnrichment(t *testing.T) {
	now := time.Now()
	hub, _ := spokeWithForgeFixture(t, now)

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Workspace) != 1 || res.Workspace[0].Err != "" {
		t.Fatalf("expected one healthy spoke, got %+v", res.Workspace)
	}
	spokePath := res.Workspace[0].Path

	fetch := func(path string) (*forge.Data, bool) {
		if path != spokePath {
			return nil, false // the hub has no reachable forge
		}
		return &forge.Data{
			Repo:   "acme/api",
			Issues: []forge.Issue{{Number: 1, Title: "Sync broken", State: "OPEN", UpdatedAt: now.AddDate(0, 0, -2)}},
			PRs:    []forge.PR{{Number: 7, State: "OPEN", HeadRefName: "feature/tb-spk1-api"}},
		}, true
	}
	EnrichWithForges(res, fetch, Options{Now: now})

	if res.Workspace[0].Forge != "acme/api" {
		t.Errorf("spoke forge = %q, want acme/api", res.Workspace[0].Forge)
	}
	if res.Forge != "" {
		t.Errorf("hub forge = %q, want empty — no forge answered for the hub", res.Forge)
	}

	u := unitByName(t, res, "feature/tb-spk1-api")
	if u.Repo != "api" || u.Status != InReview || !strings.Contains(u.Evidence, "PR #7") {
		t.Errorf("spoke branch = %s repo=%q (%s), want in-review in api citing PR #7", u.Status, u.Repo, u.Evidence)
	}

	if got := claimsByKind(res, "ticket-done-but-open"); len(got) != 1 || got[0].Subject != "api:#1" {
		t.Errorf("ticket-done-but-open = %+v, want exactly api:#1 — subjects must carry the repo", got)
	} else if !strings.Contains(got[0].Detail, "api:main") {
		t.Errorf("detail should name the spoke's integration branch, got %q", got[0].Detail)
	}
	if got := claimsByKind(res, "unticketed-work"); len(got) != 1 || got[0].Subject != "api:feature/mystery" {
		t.Errorf("unticketed-work = %+v, want exactly api:feature/mystery", got)
	}
}

// TestSpokeCIRedFlipsSpecRegressed: red CI on a spoke landing regresses
// the spec that landed there — for a plain spec and for a repos: spec via
// its per-repo landing — with the repo named in the evidence.
func TestSpokeCIRedFlipsSpecRegressed(t *testing.T) {
	now := time.Now()

	spoke := newFixture(t)
	spoke.commit("chore: init api", now.AddDate(0, 0, -10))
	for _, id := range []string{"tb-rra1", "tb-rra2"} {
		branch := "feature/" + id + "-api"
		spoke.git("checkout", "-b", branch)
		spoke.commit("feat: api half\n\nSpec: "+id, now.AddDate(0, 0, -2))
		spoke.git("checkout", "main")
		spoke.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
			"-m", "Merge branch '"+branch+"'", branch)
		spoke.git("branch", "-D", branch)
	}

	hub := newFixture(t)
	hub.commit("chore: init hub", now.AddDate(0, 0, -10))
	hub.git("checkout", "-b", "feature/tb-rra1-hub")
	hub.commit("feat: hub half\n\nSpec: tb-rra1", now.AddDate(0, 0, -2))
	hub.git("checkout", "main")
	hub.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff",
		"-m", "Merge branch 'feature/tb-rra1-hub'", "feature/tb-rra1-hub")
	hub.git("branch", "-D", "feature/tb-rra1-hub")
	writeSpecRepos(t, hub.dir, "tb-rra1", "Cross-repo story", []string{"hub", "api"})
	writeSpec(t, hub.dir, "tb-rra2", "Spoke-only story", "")
	writeWorkspace(t, hub.dir, "repos:\n  api:\n    path: "+spoke.dir+"\n    integration: main\n")

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"tb-rra1", "tb-rra2"} {
		if s := specByID(t, res, id); s.Status != Done {
			t.Fatalf("precondition: %s should be done, got %s (%s)", id, s.Status, s.Evidence)
		}
	}

	spokePath := res.Workspace[0].Path
	fetch := func(path string) (*forge.Data, bool) {
		if path != spokePath {
			return nil, false
		}
		return &forge.Data{Repo: "acme/api",
			Checks: func(string) (string, bool) { return "failure", true }}, true
	}
	EnrichWithForges(res, fetch, Options{Now: now})

	if s := specByID(t, res, "tb-rra1"); s.Status != Regressed || !strings.Contains(s.Evidence, "in api") {
		t.Errorf("repos: spec = %s (%s), want regressed naming api", s.Status, s.Evidence)
	}
	if s := specByID(t, res, "tb-rra2"); s.Status != Regressed || !strings.Contains(s.Evidence, "in api") {
		t.Errorf("plain spoke-landed spec = %s (%s), want regressed naming api", s.Status, s.Evidence)
	}
}

// TestSpokeForgeUnreachableShowsNote: a spoke without a reachable forge
// keeps its git-only derivation and carries a visible note — degraded,
// never silent, never an error.
func TestSpokeForgeUnreachableShowsNote(t *testing.T) {
	now := time.Now()
	hub, _ := spokeWithForgeFixture(t, now)

	res, err := Audit(hub.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	EnrichWithForges(res, func(string) (*forge.Data, bool) { return nil, false }, Options{Now: now})

	h := res.Workspace[0]
	if h.ForgeNote == "" || h.Forge != "" || h.Err != "" {
		t.Errorf("spoke health = %+v, want a forge note with no forge and no error", h)
	}
	if got := unitByName(t, res, "feature/tb-spk1-api"); got.Status != InProgress {
		t.Errorf("spoke branch = %s, want in-progress — git-only derivation must survive", got.Status)
	}
	if len(res.Claims) != 0 {
		t.Errorf("claims = %+v, want none — no forge, no tracker to audit", res.Claims)
	}
}
