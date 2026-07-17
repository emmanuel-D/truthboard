package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpecSprint(t *testing.T, repo, id, title, sprint string) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n"
	if sprint != "" {
		content += "sprint: " + sprint + "\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSprintRollupIsDerivedArithmetic(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	// s12: one story landed, one in progress. s9: one planned story.
	f.git("checkout", "-b", "feature/tb-s1a-done")
	f.commit("feat: done work\n\nSpec: tb-s1a", now.AddDate(0, 0, -2))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-s1a-done'", "feature/tb-s1a-done")
	f.git("branch", "-D", "feature/tb-s1a-done")
	f.git("checkout", "-b", "feature/tb-s1b-wip")
	f.commit("feat: wip\n\nSpec: tb-s1b", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	writeSpecSprint(t, f.dir, "tb-s1a", "Landed story", "s12")
	writeSpecSprint(t, f.dir, "tb-s1b", "Open story", "s12")
	writeSpecSprint(t, f.dir, "tb-s2a", "Old sprint leftover", "s9")
	writeSpecSprint(t, f.dir, "tb-s3a", "No sprint at all", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sprints) != 2 {
		t.Fatalf("sprints = %+v, want 2 rollups (sprintless spec must not appear)", res.Sprints)
	}
	// Newest sprint first: s12 before s9.
	s12 := res.Sprints[0]
	if s12.Name != "s12" || s12.Done != 1 || s12.Total != 2 {
		t.Errorf("s12 = %+v, want 1/2 done", s12)
	}
	if len(s12.Open) != 1 || s12.Open[0].ID != "tb-s1b" || s12.Open[0].Status != InProgress {
		t.Errorf("s12 open = %+v, want tb-s1b in-progress", s12.Open)
	}
	if res.Sprints[1].Name != "s9" || res.Sprints[1].Done != 0 || res.Sprints[1].Total != 1 {
		t.Errorf("s9 = %+v, want 0/1 done", res.Sprints[1])
	}
}

func TestNoSprintsNoRollup(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -1))
	writeSpecSprint(t, f.dir, "tb-nos1", "Plain story", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Sprints != nil {
		t.Errorf("sprints = %+v, want none — sprints are opt-in", res.Sprints)
	}
}
