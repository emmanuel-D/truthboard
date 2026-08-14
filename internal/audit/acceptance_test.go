package audit

import (
	"strings"
	"testing"
	"time"
)

// specWith renders a spec file whose acceptance checklist is exactly marks:
// " " for open, "x" for ticked. No checklist at all when marks is empty.
func specWith(id, title string, marks ...string) string {
	s := "---\nid: " + id + "\ntitle: " + title + "\n---\n\n## Goal\nTest.\n"
	if len(marks) == 0 {
		return s
	}
	s += "\n## Acceptance\n\n"
	for i, m := range marks {
		s += "- [" + m + "] criterion " + string(rune('A'+i)) + "\n"
	}
	return s
}

func unverifiedByID(res *Result, id string) *UnverifiedAcceptance {
	for i := range res.Drift.UnverifiedAcceptance {
		if res.Drift.UnverifiedAcceptance[i].ID == id {
			return &res.Drift.UnverifiedAcceptance[i]
		}
	}
	return nil
}

func TestUnverifiedAcceptanceIsDriftNotStatus(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	// Landed, nothing ticked — the failure this whole signal exists for.
	f.commitContents("feat: build it\n\nSpec: tb-open", now.AddDate(0, 0, -4),
		map[string]string{specFile("tb-open"): specWith("tb-open", "Landed unverified", " ", " "), "pkg/a.go": "package pkg"})
	// Landed and signed off.
	f.commitContents("feat: build it\n\nSpec: tb-tick", now.AddDate(0, 0, -3),
		map[string]string{specFile("tb-tick"): specWith("tb-tick", "Landed verified", "x", "x"), "pkg/b.go": "package pkg"})
	// Landed with no checklist at all — nothing to verify, so not drift.
	f.commitContents("feat: build it\n\nSpec: tb-bare", now.AddDate(0, 0, -2),
		map[string]string{specFile("tb-bare"): specWith("tb-bare", "No criteria"), "pkg/c.go": "package pkg"})
	// Planned: unticked, but nobody promised it yet.
	f.commitContents("Story: later\n\nSpec: tb-plan", now.AddDate(0, 0, -1),
		map[string]string{specFile("tb-plan"): specWith("tb-plan", "Not started", " ")})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	u := unverifiedByID(res, "tb-open")
	if u == nil {
		t.Fatalf("landed work with 0/2 ticked must be reported, drift = %+v", res.Drift.UnverifiedAcceptance)
	}
	if u.Done != 0 || u.Total != 2 {
		t.Errorf("counts = %d/%d, want 0/2", u.Done, u.Total)
	}
	if len(u.Unticked) != 2 || u.Unticked[0] != "criterion A" {
		t.Errorf("unticked = %v, want both criteria named", u.Unticked)
	}
	for _, id := range []string{"tb-tick", "tb-bare", "tb-plan"} {
		if unverifiedByID(res, id) != nil {
			t.Errorf("%s must not be reported as unverified", id)
		}
	}

	// The point of the whole design: this changes no status anywhere.
	if got := specByID(t, res, "tb-open"); got.Status != Done {
		t.Errorf("tb-open = %q (%s), want done — an unticked box must never gate a status", got.Status, got.Evidence)
	}
	if res.Drift.Clean() {
		t.Error("Drift.Clean() must account for unverified acceptance")
	}
}

func TestNextWarnsAboutUnverifiedAcceptanceFirst(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	f.commitContents("feat: build it\n\nSpec: tb-old", now.AddDate(0, 0, -5),
		map[string]string{specFile("tb-old"): specWith("tb-old", "Landed a while ago", " "), "pkg/a.go": "package pkg"})
	f.commitContents("feat: build it\n\nSpec: tb-new", now.AddDate(0, 0, -1),
		map[string]string{specFile("tb-new"): specWith("tb-new", "Landed just now", " ", "x"), "pkg/b.go": "package pkg"})
	f.commitContents("Story: next up\n\nSpec: tb-todo", now,
		map[string]string{specFile("tb-todo"): specWith("tb-todo", "Still planned", " ")})

	up, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if up.Spec == nil || up.Spec.ID != "tb-todo" {
		t.Fatalf("Next = %+v, want tb-todo", up.Spec)
	}
	if len(up.Unverified) != 2 {
		t.Fatalf("got %d unverified, want 2: %+v", len(up.Unverified), up.Unverified)
	}
	// Newest landing first: the story the agent just finished is the one it
	// still has the context to verify.
	if up.Unverified[0].ID != "tb-new" {
		t.Errorf("unverified order = %s first, want tb-new (newest landing)", up.Unverified[0].ID)
	}

	msg := SignoffReminder(up.Unverified)
	for _, want := range []string{"tb-new", "1 of 2 criteria ticked", "truthboard check tb-new all", "1 more"} {
		if !strings.Contains(msg, want) {
			t.Errorf("reminder %q missing %q", msg, want)
		}
	}
	if SignoffReminder(nil) != "" {
		t.Error("an empty reminder must stay empty so callers can print it unconditionally")
	}
}

func TestBriefNumbersTheChecklistAndAsksForTheTick(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -2))
	f.commitContents("Story: do it\n\nSpec: tb-work", now.AddDate(0, 0, -1),
		map[string]string{specFile("tb-work"): specWith("tb-work", "Work to do", " ", "x")})

	text, err := Brief(f.dir, "tb-work")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"1. [ ] criterion A",
		"2. [x] criterion B",
		"check_acceptance",
		"truthboard check tb-work",
		"Do not tick what you did not verify",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("brief missing %q:\n%s", want, text)
		}
	}

	// A spec with no checklist gets no instructions about one.
	f.commitContents("Story: bare\n\nSpec: tb-bare", now,
		map[string]string{specFile("tb-bare"): specWith("tb-bare", "No criteria")})
	text, err = Brief(f.dir, "tb-bare")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "check_acceptance") {
		t.Errorf("a spec with no checklist must not be told to tick one:\n%s", text)
	}
}
