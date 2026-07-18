package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpecNeeds(t *testing.T, repo, id, title string, priority int, needs ...string) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n"
	if priority > 0 {
		content += "priority: " + string(rune('0'+priority)) + "\n"
	}
	if len(needs) > 0 {
		content += "needs: [" + strings.Join(needs, ", ") + "]\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWaitingIsDerivedFromNeeds(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	// tb-base landed; tb-open did not.
	f.git("checkout", "-b", "feature/tb-base-work")
	f.commit("feat: base\n\nSpec: tb-base", now.AddDate(0, 0, -2))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-base-work'", "feature/tb-base-work")
	f.git("branch", "-D", "feature/tb-base-work")

	writeSpecNeeds(t, f.dir, "tb-base", "Foundation", 1)
	writeSpecNeeds(t, f.dir, "tb-open", "Open prerequisite", 1)
	writeSpecNeeds(t, f.dir, "tb-redy", "Ready story", 2, "tb-base")
	writeSpecNeeds(t, f.dir, "tb-wait", "Waiting story", 1, "tb-base", "tb-open")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]SpecStatus{}
	for _, s := range res.Specs {
		byID[s.ID] = s
	}
	// A done need is met; only the open one blocks.
	if w := byID["tb-wait"].Waiting; len(w) != 1 || w[0] != "tb-open" {
		t.Errorf("tb-wait waiting = %v, want [tb-open]", w)
	}
	if !strings.Contains(byID["tb-wait"].Evidence, "waiting on tb-open") {
		t.Errorf("tb-wait evidence = %q, want the waiting note", byID["tb-wait"].Evidence)
	}
	if w := byID["tb-redy"].Waiting; len(w) != 0 {
		t.Errorf("tb-redy waiting = %v, want none (its need is done)", w)
	}
	if len(res.Drift.DependencyCycles) != 0 {
		t.Errorf("cycles = %v, want none", res.Drift.DependencyCycles)
	}
}

func TestNextSkipsWaitingStories(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -3))

	// tb-wait is top priority but waits on tb-late; tb-free is startable.
	writeSpecNeeds(t, f.dir, "tb-late", "The prerequisite", 3)
	writeSpecNeeds(t, f.dir, "tb-wait", "Blocked top priority", 1, "tb-late")
	writeSpecNeeds(t, f.dir, "tb-free", "Startable", 2)
	f.commit("chore: backlog", now.AddDate(0, 0, -1))

	next, _, waiting, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID == "tb-wait" {
		t.Fatalf("Next = %+v — an agent must never be handed a waiting story", next)
	}
	found := false
	for _, w := range waiting {
		if w.ID == "tb-wait" {
			found = true
		}
	}
	if !found {
		t.Errorf("waiting list %v does not name tb-wait", waiting)
	}
}

func TestDependencyCycleIsALoudFinding(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -3))
	writeSpecNeeds(t, f.dir, "tb-ca", "First of the loop", 1, "tb-cb")
	writeSpecNeeds(t, f.dir, "tb-cb", "Second of the loop", 1, "tb-ca")
	f.commit("chore: backlog", now.AddDate(0, 0, -1))

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.DependencyCycles) != 1 {
		t.Fatalf("cycles = %v, want exactly one", res.Drift.DependencyCycles)
	}
	cy := res.Drift.DependencyCycles[0]
	if !strings.Contains(cy, "tb-ca") || !strings.Contains(cy, "tb-cb") || !strings.Contains(cy, "→") {
		t.Errorf("cycle = %q, want a rendered chain naming both specs", cy)
	}
	// Nothing in the cycle is ever startable.
	next, _, _, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Errorf("Next = %+v, want nil — cycle members must wait, loudly, not run", next)
	}
}

func TestUnknownNeedStaysVisible(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -3))
	writeSpecNeeds(t, f.dir, "tb-orph", "References a ghost", 1, "tb-gone")
	f.commit("chore: backlog", now.AddDate(0, 0, -1))

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if w := res.Specs[0].Waiting; len(w) != 1 || w[0] != "tb-gone?" {
		t.Errorf("waiting = %v, want the ghost marked tb-gone?", w)
	}
}
