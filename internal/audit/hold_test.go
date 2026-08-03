package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpecHold(t *testing.T, repo, id, title, hold string) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n"
	if hold != "" {
		content += "hold: " + hold + "\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findSpec(t *testing.T, res *Result, id string) SpecStatus {
	t.Helper()
	for _, s := range res.Specs {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("spec %s not in result", id)
	return SpecStatus{}
}

// A hold explains a pause. On work that is stalled or planned it stands;
// git has nothing to say against it.
func TestHoldStandsOnPausedWork(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -40))
	f.git("checkout", "-b", "feature/tb-h001-stuck")
	f.commit("wip: started then stopped\n\nSpec: tb-h001", now.AddDate(0, 0, -30))
	f.git("checkout", "main")

	writeSpecHold(t, f.dir, "tb-h001", "Stalled with a reason", "waiting on legal sign-off")
	writeSpecHold(t, f.dir, "tb-h002", "Never started", "deprioritised for Q3")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	stalled := findSpec(t, res, "tb-h001")
	if stalled.Status != Stalled {
		t.Fatalf("status = %s, want stalled — a hold must not change what git derives", stalled.Status)
	}
	if stalled.Hold != "waiting on legal sign-off" {
		t.Errorf("hold = %q, want it carried through to SpecStatus", stalled.Hold)
	}
	if stalled.HoldContradicted != "" {
		t.Errorf("hold on stalled work contradicted (%q); the pause is exactly what the note explains", stalled.HoldContradicted)
	}
	if planned := findSpec(t, res, "tb-h002"); planned.Status != Planned || planned.HoldContradicted != "" {
		t.Errorf("planned story: status=%s contradicted=%q, want planned and uncontradicted", planned.Status, planned.HoldContradicted)
	}
	if n := len(res.Drift.ContradictedHolds); n != 0 {
		t.Errorf("drift reports %d contradicted holds, want 0", n)
	}
}

// The failure mode this whole field guards against: someone writes a true
// reason, the work resumes, and nobody deletes the note.
func TestHoldIsContradictedWhenWorkLanded(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.git("checkout", "-b", "feature/tb-h003-shipped")
	f.commit("feat: shipped anyway\n\nSpec: tb-h003", now.AddDate(0, 0, -2))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-h003-shipped'", "feature/tb-h003-shipped")

	writeSpecHold(t, f.dir, "tb-h003", "Shipped while held", "waiting on the vendor")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := findSpec(t, res, "tb-h003")
	if s.Status != Done {
		t.Fatalf("status = %s, want done — a hold never blocks a derived status", s.Status)
	}
	if s.HoldContradicted == "" {
		t.Fatal("a hold on landed work was not contradicted; the board would repeat a reason that stopped being true")
	}
	if len(res.Drift.ContradictedHolds) != 1 {
		t.Fatalf("drift reports %d contradicted holds, want 1", len(res.Drift.ContradictedHolds))
	}
	got := res.Drift.ContradictedHolds[0]
	if got.ID != "tb-h003" || got.Hold != "waiting on the vendor" || got.Why == "" {
		t.Errorf("drift entry = %+v, want the id, the note and the evidence against it", got)
	}
}

func TestHoldIsContradictedWhenWorkIsMoving(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.git("checkout", "-b", "feature/tb-h004-active")
	f.commit("wip: actually working on it\n\nSpec: tb-h004", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	writeSpecHold(t, f.dir, "tb-h004", "Held but moving", "blocked on design")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := findSpec(t, res, "tb-h004")
	if s.Status != InProgress {
		t.Fatalf("status = %s, want in-progress", s.Status)
	}
	if s.HoldContradicted == "" {
		t.Error("a hold on work with fresh commits was not contradicted")
	}
}

// A repo that never writes a hold must see nothing new anywhere.
func TestNoHoldsMeansNoNewOutput(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	writeSpecHold(t, f.dir, "tb-h005", "Ordinary story", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := findSpec(t, res, "tb-h005")
	if s.Hold != "" || s.HoldContradicted != "" {
		t.Errorf("spec without a hold carries %q/%q, want both empty", s.Hold, s.HoldContradicted)
	}
	if res.Drift.ContradictedHolds != nil {
		t.Errorf("drift carries %v, want nil so --format json omits the key entirely", res.Drift.ContradictedHolds)
	}
}

// Clearing a hold is deleting the line — there is no unhold verb.
func TestClearingAHoldIsDeletingTheLine(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	writeSpecHold(t, f.dir, "tb-h006", "Was held", "waiting on legal")

	s, err := spec.Find(f.dir, "tb-h006")
	if err != nil {
		t.Fatal(err)
	}
	s.Hold = ""
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.File)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); contains(got, "hold:") {
		t.Errorf("cleared hold still in the file:\n%s", got)
	}
	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if h := findSpec(t, res, "tb-h006").Hold; h != "" {
		t.Errorf("hold = %q after clearing, want empty", h)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
