package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpecPri(t *testing.T, repo, id, title string, priority int) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n"
	if priority > 0 {
		content += "priority: " + string(rune('0'+priority)) + "\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNextPicksHighestPriorityPlannedOnly(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -3))

	// tb-busy is priority 1 but already has a branch — never "next".
	f.git("checkout", "-b", "feature/tb-busy-work")
	f.commit("feat: claimed", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	writeSpecPri(t, f.dir, "tb-busy", "Claimed top priority", 1)
	writeSpecPri(t, f.dir, "tb-cccc", "Planned, priority 2", 2)
	writeSpecPri(t, f.dir, "tb-bbbb", "Planned, priority 1", 1)
	writeSpecPri(t, f.dir, "tb-aaaa", "Planned, no priority", 0)
	// Land the backlog on main so branch switches below don't carry the
	// (otherwise uncommitted) spec files away with them.
	f.commit("chore: backlog", now.AddDate(0, 0, -2))

	next, _, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "tb-bbbb" {
		t.Fatalf("Next = %+v, want tb-bbbb (highest-priority planned)", next)
	}

	// The moment someone claims it with a branch, the answer moves on —
	// two agents asking in sequence never get the same story.
	f.git("checkout", "-b", "feature/tb-bbbb-claimed")
	f.commit("feat: claimed too", now)
	f.git("checkout", "main")

	next, _, err = Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.ID != "tb-cccc" {
		t.Fatalf("Next after claim = %+v, want tb-cccc", next)
	}
}

func TestNextEmptyBacklogReportsStalled(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -40))

	// One story stalled (branch stopped 30 days ago), none planned.
	f.git("checkout", "-b", "feature/tb-old-work")
	f.commit("feat: abandoned", now.AddDate(0, 0, -30))
	f.git("checkout", "main")
	writeSpecPri(t, f.dir, "tb-old", "Abandoned work", 1)

	next, stalled, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if next != nil {
		t.Fatalf("Next = %+v, want nil (nothing planned)", next)
	}
	if stalled != 1 {
		t.Errorf("stalled = %d, want 1", stalled)
	}
}
