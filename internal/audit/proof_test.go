package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// specProved renders a spec whose criteria carry evidence clauses. Each
// entry is "mark|text — proof: ref", or just "mark|text" for a bare tick.
func specProved(id, title string, lines ...string) string {
	s := "---\nid: " + id + "\ntitle: " + title + "\n---\n\n## Goal\nTest.\n\n## Acceptance\n\n"
	for _, l := range lines {
		mark, text, _ := strings.Cut(l, "|")
		s += "- [" + mark + "] " + text + "\n"
	}
	return s
}

func brokenByID(res *Result, id string) *BrokenProof {
	for i := range res.Drift.BrokenProofs {
		if res.Drift.BrokenProofs[i].ID == id {
			return &res.Drift.BrokenProofs[i]
		}
	}
	return nil
}

// TestEvidenceIsRederivedNotRemembered is the whole point: a tick that
// names its proof is re-checked on every audit, so the claim cannot outlive
// the thing that supported it.
func TestEvidenceIsRederivedNotRemembered(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	f.commitContents("Deliver with evidence\n\nSpec: tb-ev", now.AddDate(0, 0, -5), map[string]string{
		specFile("tb-ev"): specProved("tb-ev", "Proved",
			"x|the file is there — proof: `pkg/kept.go`",
			"x|the name is there — proof: `TestSomethingReal`",
			"x|the pipeline is green — proof: `ci:build`",
			"x|a prose promise nobody can automate"),
		"pkg/kept.go":      "package pkg",
		"pkg/kept_test.go": "package pkg\n\nfunc TestSomethingReal(t *testing.T) {}\n",
	})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if b := brokenByID(res, "tb-ev"); b != nil {
		t.Errorf("evidence that is all present must not be drift: %+v", b)
	}
	// The CI check cannot be seen from a checkout — reported, but as a
	// disclosure rather than a finding.
	if len(res.Drift.UncheckedProofs) != 1 || res.Drift.UncheckedProofs[0].Ref != "ci:build" {
		t.Errorf("unchecked = %+v, want the ci: check named", res.Drift.UncheckedProofs)
	}
	// A forge check nobody can see from here is a disclosure, not a finding:
	// it must not turn up among the broken ones, and must not count against
	// a clean board.
	if len(res.Drift.BrokenProofs) != 0 {
		t.Errorf("unverifiable evidence must not be reported as broken: %+v", res.Drift.BrokenProofs)
	}
	if !(Drift{UncheckedProofs: res.Drift.UncheckedProofs}).Clean() {
		t.Error("a board whose only finding is an unseeable forge check reads as clean")
	}
	if got := specByID(t, res, "tb-ev").AcceptanceProved; got != 3 {
		t.Errorf("proved count = %d, want the 3 criteria carrying evidence", got)
	}

	// Now delete what the evidence pointed at. Nothing about the spec file
	// changes — the claim is identical — and the audit must notice anyway.
	f.commitContents("Remove the test and the file it covered", now.AddDate(0, 0, -1),
		map[string]string{"pkg/other.go": "package pkg"})
	f.git("rm", "-q", "pkg/kept.go", "pkg/kept_test.go")
	f.gitAt(now, "commit", "-m", "Delete the evidence")

	after, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Drift.BrokenProofs) != 2 {
		t.Fatalf("broken = %+v, want the missing path and the missing test", after.Drift.BrokenProofs)
	}
	var refs []string
	for _, b := range after.Drift.BrokenProofs {
		refs = append(refs, b.Ref)
		if b.Criterion == "" || b.N == 0 {
			t.Errorf("a finding must name the criterion it belongs to: %+v", b)
		}
	}
	if strings.Join(refs, ",") != "pkg/kept.go,TestSomethingReal" {
		t.Errorf("refs = %v, want both the path and the test named", refs)
	}
	if after.Drift.Clean() {
		t.Error("broken evidence is drift")
	}

	// And the status is untouched: git alone still derives it.
	if got := specByID(t, after, "tb-ev").Status; got != Done {
		t.Errorf("status = %q, want done — evidence never gates a status", got)
	}
}

// TestEvidenceIsOptional guards the design's limit: prose criteria are the
// reason acceptance is a human claim, and a scheme that only accepted
// machine-checkable promises would push them out of the checklist.
func TestEvidenceIsOptional(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Deliver\n\nSpec: tb-bare", now.AddDate(0, 0, -2), map[string]string{
		specFile("tb-bare"): specProved("tb-bare", "Bare ticks", "x|a PO can read this", " |not done yet"),
		"pkg/a.go":          "package pkg",
	})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.BrokenProofs) != 0 || len(res.Drift.UncheckedProofs) != 0 {
		t.Errorf("a checklist with no evidence must produce no proof findings: %+v %+v",
			res.Drift.BrokenProofs, res.Drift.UncheckedProofs)
	}
	if got := specByID(t, res, "tb-bare").AcceptanceProved; got != 0 {
		t.Errorf("proved = %d, want 0", got)
	}
}

// TestEvidenceOnAnUntickedCriterionIsNotAClaim: naming what will prove a
// promise is a plan. Only a tick asserts anything.
func TestEvidenceOnAnUntickedCriterionIsNotAClaim(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Deliver\n\nSpec: tb-plan", now.AddDate(0, 0, -2), map[string]string{
		specFile("tb-plan"): specProved("tb-plan", "Planned proof", " |will be proved by — proof: `TestThatDoesNotExistYet`"),
		"pkg/a.go":          "package pkg",
	})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.BrokenProofs) != 0 {
		t.Errorf("evidence named on an unticked criterion is a plan, not a broken claim: %+v", res.Drift.BrokenProofs)
	}
}

// TestEvidenceSeesWorkNotYetCommitted: the test that proves a criterion is
// usually written in the same session as the tick. Telling someone their
// brand-new test does not exist, until they commit, would teach them the
// check is noise.
func TestEvidenceSeesWorkNotYetCommitted(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Deliver\n\nSpec: tb-fresh", now.AddDate(0, 0, -2), map[string]string{
		specFile("tb-fresh"): specProved("tb-fresh", "Fresh", "x|proved by a new test — proof: `TestWrittenJustNow`"),
		"pkg/a.go":           "package pkg",
	})
	// Written, not committed — exactly where an agent is when it ticks.
	if err := os.WriteFile(filepath.Join(f.dir, "pkg", "new_test.go"),
		[]byte("package pkg\n\nfunc TestWrittenJustNow(t *testing.T) {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if b := brokenByID(res, "tb-fresh"); b != nil {
		t.Errorf("uncommitted work is still work: %+v", b)
	}
}
