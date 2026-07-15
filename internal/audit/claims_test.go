package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/forge"
)

// buildClaimsFixture: main carries a merged "fixes #1" commit; feature/ticketed
// references #2 in an unmerged commit; feature/orphan references nothing.
func buildClaimsFixture(t *testing.T, now time.Time) *fixture {
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))

	f.git("checkout", "-b", "feature/fixer")
	f.commit("feat: repair login, fixes #1", now.AddDate(0, 0, -3))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -3), "merge", "--no-ff", "-m", "Merge branch 'feature/fixer'", "feature/fixer")
	f.git("branch", "-D", "feature/fixer")

	f.git("checkout", "-b", "feature/ticketed", "main")
	f.commit("feat: progress on #2", now.AddDate(0, 0, -1))

	f.git("checkout", "-b", "feature/orphan", "main")
	f.commit("feat: mystery work", now.AddDate(0, 0, -1))

	// References #99 which exists in no tracker — incidental #N strings
	// (milestones, "item #2") must not make a branch count as ticketed.
	f.git("checkout", "-b", "feature/phantom-ref", "main")
	f.commit("feat: level #99 polish", now.AddDate(0, 0, -1))
	f.git("checkout", "main")
	return f
}

func claimsByKind(res *Result, kind string) []Claim {
	var out []Claim
	for _, c := range res.Claims {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

func TestClaimsVsProof(t *testing.T) {
	now := time.Now()
	f := buildClaimsFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	assigned := []forge.Assignee{{Login: "emmanuel"}}
	data := &forge.Data{
		Repo: "test/fixture",
		Issues: []forge.Issue{
			{Number: 1, Title: "Login broken", State: "OPEN", UpdatedAt: now.AddDate(0, 0, -2)},
			{Number: 2, Title: "Active work", State: "OPEN", UpdatedAt: now.AddDate(0, 0, -1), Assignees: assigned},
			{Number: 3, Title: "Claimed then forgotten", State: "OPEN", UpdatedAt: now.AddDate(0, 0, -40), Assignees: assigned},
			{Number: 4, Title: "Already closed", State: "CLOSED", UpdatedAt: now.AddDate(0, 0, -40)},
			{Number: 5, Title: "Unassigned backlog wish", State: "OPEN", UpdatedAt: now.AddDate(0, 0, -300)},
		},
	}
	EnrichWithForge(res, data, Options{Now: now})

	if got := claimsByKind(res, "ticket-done-but-open"); len(got) != 1 || got[0].Subject != "#1" {
		t.Errorf("ticket-done-but-open = %+v, want exactly #1", got)
	}
	if got := claimsByKind(res, "ticket-stale"); len(got) != 1 || got[0].Subject != "#3" {
		t.Errorf("ticket-stale = %+v, want exactly #3 (not #2: has activity; not #4: closed; not #5: unassigned backlog)", got)
	}
	if got := claimsByKind(res, "unticketed-work"); len(got) != 2 ||
		got[0].Subject != "feature/orphan" || got[1].Subject != "feature/phantom-ref" {
		t.Errorf("unticketed-work = %+v, want feature/orphan and feature/phantom-ref (phantom #99 must not count)", got)
	}
	if res.Forge != "test/fixture" {
		t.Errorf("forge = %q, want test/fixture", res.Forge)
	}
}

func TestPRStateUpgradesUnits(t *testing.T) {
	now := time.Now()
	f := buildClaimsFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	data := &forge.Data{
		Repo: "test/fixture",
		PRs: []forge.PR{
			{Number: 10, State: "OPEN", HeadRefName: "feature/ticketed"},
			{Number: 11, State: "CLOSED", HeadRefName: "feature/orphan"},
		},
	}
	EnrichWithForge(res, data, Options{Now: now})

	if got := unitByName(t, res, "feature/ticketed"); got.Status != InReview || !strings.Contains(got.Evidence, "PR #10") {
		t.Errorf("feature/ticketed = %q (%s), want in-review citing PR #10", got.Status, got.Evidence)
	}
	if got := claimsByKind(res, "pr-abandoned"); len(got) != 1 || got[0].Subject != "feature/orphan" {
		t.Errorf("pr-abandoned = %+v, want feature/orphan", got)
	}
	// A branch with a PR is promised work even without issue refs; only the
	// PR-less phantom-ref branch stays unticketed.
	if got := claimsByKind(res, "unticketed-work"); len(got) != 1 || got[0].Subject != "feature/phantom-ref" {
		t.Errorf("unticketed-work = %+v, want exactly feature/phantom-ref", got)
	}
}

func TestForgeNeverDowngradesGitEvidence(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	// Tracker claims a PR is open on a branch git already proved merged.
	data := &forge.Data{
		Repo: "test/fixture",
		PRs:  []forge.PR{{Number: 12, State: "OPEN", HeadRefName: "feature/merged"}},
	}
	EnrichWithForge(res, data, Options{Now: now})

	if got := unitByName(t, res, "feature/merged").Status; got != Done {
		t.Errorf("feature/merged = %q, want done — git proof outranks tracker claims", got)
	}
}
