package audit

import (
	"fmt"
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

func writeSpecPoints(t *testing.T, repo, id, title, sprint string, points int) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\nsprint: " + sprint + "\n"
	if points > 0 {
		content += fmt.Sprintf("points: %d\n", points)
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSprintPointRollup(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	f.git("checkout", "-b", "feature/tb-p1a-done")
	f.commit("feat: done work\n\nSpec: tb-p1a", now.AddDate(0, 0, -2))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -2), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-p1a-done'", "feature/tb-p1a-done")
	f.git("branch", "-D", "feature/tb-p1a-done")

	writeSpecPoints(t, f.dir, "tb-p1a", "Landed, 5pt", "s12", 5)
	writeSpecPoints(t, f.dir, "tb-p1b", "Open, 3pt", "s12", 3)
	writeSpecPoints(t, f.dir, "tb-p1c", "Open, unestimated", "s12", 0)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sprints) != 1 {
		t.Fatalf("sprints = %+v, want just s12", res.Sprints)
	}
	sp := res.Sprints[0]
	if sp.PointsDone != 5 || sp.PointsTotal != 8 || sp.Unestimated != 1 {
		t.Errorf("points = %d/%d, %d unestimated; want 5/8 with 1 unestimated (never counted as zero)",
			sp.PointsDone, sp.PointsTotal, sp.Unestimated)
	}
	if sp.Done != 1 || sp.Total != 3 {
		t.Errorf("arithmetic = %d/%d, points must not disturb story counts", sp.Done, sp.Total)
	}
}

func writeSprintFile(t *testing.T, repo, slug, start, end string) {
	t.Helper()
	dir := spec.SprintDir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nslug: " + slug + "\n"
	if start != "" {
		content += "start: " + start + "\n"
	}
	if end != "" {
		content += "end: " + end + "\n"
	}
	content += "---\n"
	if err := os.WriteFile(filepath.Join(dir, slug+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSprintDatesDeriveState(t *testing.T) {
	now := time.Date(2026, 7, 17, 15, 0, 0, 0, time.UTC)
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	writeSpecSprint(t, f.dir, "tb-da1", "Active story", "s12")
	writeSprintFile(t, f.dir, "s12", "2026-07-14", "2026-07-25")
	writeSprintFile(t, f.dir, "s11", "2026-06-29", "2026-07-10")
	writeSprintFile(t, f.dir, "s13", "2026-07-28", "2026-08-08")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SprintRollup{}
	for _, sp := range res.Sprints {
		got[sp.Name] = sp
	}
	// s12 has a story and a window it sits inside: active, 8 days to the
	// inclusive end date.
	s12 := got["s12"]
	if s12.State != "active" || s12.DaysLeft != 8 || s12.Start != "2026-07-14" || s12.End != "2026-07-25" {
		t.Errorf("s12 = %+v, want active with 8d left and its window", s12)
	}
	if s12.Done != 0 || s12.Total != 1 {
		t.Errorf("s12 arithmetic = %d/%d, dates must not disturb it", s12.Done, s12.Total)
	}
	// Dated sprints appear even before stories are pulled into them.
	if got["s11"].State != "completed" {
		t.Errorf("s11 = %+v, want completed (window elapsed)", got["s11"])
	}
	if got["s13"].State != "future" {
		t.Errorf("s13 = %+v, want future (window not started)", got["s13"])
	}
}

func TestSprintEndDateIsInclusive(t *testing.T) {
	now := time.Date(2026, 7, 25, 23, 0, 0, 0, time.UTC)
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	writeSprintFile(t, f.dir, "s12", "2026-07-14", "2026-07-25")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sprints) != 1 || res.Sprints[0].State != "active" || res.Sprints[0].DaysLeft != 0 {
		t.Errorf("sprints = %+v, want s12 active with 0d left on its last day", res.Sprints)
	}
}

func TestSprintFileWithBadDateFailsLoudly(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -1))
	writeSprintFile(t, f.dir, "s12", "July 14", "")

	if _, err := Audit(f.dir, Options{Now: now}); err == nil {
		t.Error("want an error for a non-YYYY-MM-DD sprint date, got none")
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
