package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/forge"
)

// buildLandedSpecFixture: tb-r1's work landed on main via a trailer commit.
func buildLandedSpecFixture(t *testing.T, now time.Time) *fixture {
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	f.git("checkout", "-b", "feature/tb-r1-work")
	f.commit("feat: the work\n\nSpec: tb-r1", now.AddDate(0, 0, -5))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -5), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-r1-work'", "feature/tb-r1-work")
	f.git("branch", "-D", "feature/tb-r1-work")
	writeSpec(t, f.dir, "tb-r1", "Reverted work", "")
	return f
}

func TestRevertFlipsDoneToRegressed(t *testing.T) {
	now := time.Now()
	f := buildLandedSpecFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := specByID(t, res, "tb-r1"); got.Status != Done {
		t.Fatalf("before revert: status = %q, want done", got.Status)
	}

	// Revert the trailer commit (git revert of the feature commit).
	sha := f.git("log", "--grep", "Spec: tb-r1", "--format=%H", "-n", "1", "main")
	f.gitAt(now, "revert", "--no-edit", sha)

	res, err = Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	got := specByID(t, res, "tb-r1")
	if got.Status != Regressed {
		t.Errorf("after revert: status = %q, want regressed (evidence: %s)", got.Status, got.Evidence)
	}
	if !strings.Contains(got.Evidence, "reverted") {
		t.Errorf("evidence should name the revert, got %q", got.Evidence)
	}
}

func TestRevertOfMergeCommitFlipsToRegressed(t *testing.T) {
	now := time.Now()
	f := buildLandedSpecFixture(t, now)

	// Revert the merge itself (-m 1), whose subject names the spec branch.
	mergeSHA := f.git("log", "--merges", "--format=%H", "-n", "1", "main")
	f.gitAt(now, "revert", "--no-edit", "-m", "1", mergeSHA)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := specByID(t, res, "tb-r1"); got.Status != Regressed {
		t.Errorf("after merge revert: status = %q, want regressed (evidence: %s)", got.Status, got.Evidence)
	}
}

func TestRedCIFlipsDoneToRegressed(t *testing.T) {
	now := time.Now()
	f := buildLandedSpecFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	landed := specByID(t, res, "tb-r1").Landed
	if landed == "" {
		t.Fatal("expected a landed SHA for tb-r1")
	}

	var asked string
	data := &forge.Data{Repo: "test/fixture", Checks: func(sha string) (string, bool) {
		asked = sha
		return "failure", true
	}}
	EnrichWithForge(res, data, Options{Now: now})

	got := specByID(t, res, "tb-r1")
	if got.Status != Regressed || !strings.Contains(got.Evidence, "CI is red") {
		t.Errorf("with red CI: status = %q (%s), want regressed citing CI", got.Status, got.Evidence)
	}
	if asked != landed {
		t.Errorf("CI was asked about %q, want the landing commit %q", asked, landed)
	}
}

func TestNoCIDataSaysNothing(t *testing.T) {
	now := time.Now()
	f := buildLandedSpecFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	// Forge reachable but no Checks capability, then Checks that can't
	// answer, then a pending pipeline: all must leave done untouched.
	EnrichWithForge(res, &forge.Data{Repo: "test/fixture"}, Options{Now: now})
	if got := specByID(t, res, "tb-r1"); got.Status != Done {
		t.Errorf("nil Checks: status = %q, want done (say nothing without data)", got.Status)
	}

	EnrichWithForge(res, &forge.Data{Repo: "test/fixture",
		Checks: func(string) (string, bool) { return "", false }}, Options{Now: now})
	if got := specByID(t, res, "tb-r1"); got.Status != Done {
		t.Errorf("unanswerable Checks: status = %q, want done", got.Status)
	}

	EnrichWithForge(res, &forge.Data{Repo: "test/fixture",
		Checks: func(string) (string, bool) { return "pending", true }}, Options{Now: now})
	if got := specByID(t, res, "tb-r1"); got.Status != Done {
		t.Errorf("pending CI: status = %q, want done", got.Status)
	}
}
