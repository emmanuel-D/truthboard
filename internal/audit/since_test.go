package audit

import (
	"strings"
	"testing"
	"time"
)

func changeIDs(cs []Change) string {
	var ids []string
	for _, c := range cs {
		ids = append(ids, c.ID)
	}
	return strings.Join(ids, ",")
}

// TestSinceReportsWhatCommitsRemember is the standup question: what changed
// while I was away. Every answer here is recomputed from two commits, so
// nothing had to be running and nothing was stored in between.
func TestSinceReportsWhatCommitsRemember(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))

	// The state we will ask "since" about: one story filed, one landed and
	// signed off, one landed with its acceptance unread.
	f.commitContents("Story: already here\n\nSpec: tb-old", now.AddDate(0, 0, -20),
		map[string]string{specFile("tb-old"): specWith("tb-old", "Filed long ago", " ", " ")})
	f.commitContents("Deliver the unread one\n\nSpec: tb-unread", now.AddDate(0, 0, -19),
		map[string]string{specFile("tb-unread"): specWith("tb-unread", "Landed unread", " ", " "), "pkg/u.go": "package pkg"})
	mark := f.git("rev-parse", "HEAD")

	// Everything after the mark is what `since` must report.
	f.commitContents("Deliver the old one\n\nSpec: tb-old", now.AddDate(0, 0, -10),
		map[string]string{"pkg/o.go": "package pkg"})
	f.commitContents("Tick the old one\n\nSpec: tb-old", now.AddDate(0, 0, -9),
		map[string]string{specFile("tb-old"): specWith("tb-old", "Filed long ago", "x", "x")})
	f.commitContents("Story: brand new\n\nSpec: tb-new", now.AddDate(0, 0, -8),
		map[string]string{specFile("tb-new"): specWith("tb-new", "Filed since", " ")})

	d, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}

	if got := changeIDs(d.Landed); got != "tb-old" {
		t.Errorf("landed = %q, want tb-old", got)
	}
	if got := changeIDs(d.Filed); got != "tb-new" {
		t.Errorf("filed = %q, want tb-new", got)
	}
	if got := changeIDs(d.Ticked); got != "tb-old" {
		t.Errorf("ticked = %q, want tb-old", got)
	}
	if len(d.Ticked) > 0 && !strings.Contains(d.Ticked[0].Detail, "0/2 → 2/2") {
		t.Errorf("tick detail = %q, want the counts on both sides", d.Ticked[0].Detail)
	}
	// tb-unread landed *before* the mark and is unchanged: a window must not
	// re-report standing state as news.
	for _, ch := range append(append([]Change{}, d.Landed...), d.Unverified...) {
		if ch.ID == "tb-unread" {
			t.Errorf("tb-unread was already in that state at the mark — not news")
		}
	}
	if d.Quiet() {
		t.Error("a window with three changes is not quiet")
	}
	if !strings.Contains(d.Headline(), "1 story landed") {
		t.Errorf("headline = %q", d.Headline())
	}
}

// TestSinceIsQuietWhenNothingHappened: a quiet window is an answer, and the
// reason a scheduled digest can stay silent rather than posting nothing.
func TestSinceIsQuietWhenNothingHappened(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Deliver\n\nSpec: tb-a", now.AddDate(0, 0, -5),
		map[string]string{specFile("tb-a"): specWith("tb-a", "A", "x"), "pkg/a.go": "package pkg"})
	mark := f.git("rev-parse", "HEAD")
	f.commit("chore: unrelated churn nobody tracks", now.AddDate(0, 0, -1))

	d, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Quiet() {
		t.Errorf("commits that touch no story must leave the board quiet: %+v", d)
	}
	if !strings.Contains(d.Headline(), "nothing changed") {
		t.Errorf("headline = %q, want it to say so plainly", d.Headline())
	}
}

// TestSinceNeedsNoStoredState is the property that makes this derivable
// rather than remembered: same repo, same argument, same answer — from a
// process that has never run before and keeps nothing afterwards.
func TestSinceNeedsNoStoredState(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Story: one\n\nSpec: tb-one", now.AddDate(0, 0, -6),
		map[string]string{specFile("tb-one"): specWith("tb-one", "One", " ")})
	mark := f.git("rev-parse", "HEAD")
	f.commitContents("Deliver one\n\nSpec: tb-one", now.AddDate(0, 0, -2),
		map[string]string{"pkg/a.go": "package pkg"})

	first, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}
	if first.Headline() != second.Headline() || changeIDs(first.Landed) != changeIDs(second.Landed) {
		t.Errorf("two runs disagreed:\n%s\n%s", first.Headline(), second.Headline())
	}
	// Asking again after asking once must not consume the answer.
	if len(second.Landed) != 1 {
		t.Errorf("second run reported %d landed, want 1 — the diff is derived, not drained", len(second.Landed))
	}
}

// TestSinceTakesADate covers the form a person actually types.
func TestSinceTakesADate(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -40))
	f.commitContents("Deliver\n\nSpec: tb-d", now.AddDate(0, 0, -3),
		map[string]string{specFile("tb-d"): specWith("tb-d", "Recent", "x"), "pkg/a.go": "package pkg"})

	d, err := SinceDiff(f.dir, now.AddDate(0, 0, -20).Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	if changeIDs(d.Landed) != "tb-d" {
		t.Errorf("landed since a date = %q, want tb-d", changeIDs(d.Landed))
	}
	if _, err := SinceDiff(f.dir, "not-a-ref-or-a-date"); err == nil {
		t.Error("an unresolvable argument must fail loudly")
	}
}

// TestSinceReportsUnreadAcceptance is what a scheduled digest is for: work
// that landed and whose promises nobody has read back.
func TestSinceReportsUnreadAcceptance(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -20))
	f.commitContents("Story: filed\n\nSpec: tb-x", now.AddDate(0, 0, -15),
		map[string]string{specFile("tb-x"): specWith("tb-x", "Unread when it landed", " ", " ", " ")})
	mark := f.git("rev-parse", "HEAD")
	f.commitContents("Deliver it\n\nSpec: tb-x", now.AddDate(0, 0, -4),
		map[string]string{"pkg/x.go": "package pkg"})

	d, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}
	if changeIDs(d.Unverified) != "tb-x" {
		t.Fatalf("unverified = %q, want tb-x", changeIDs(d.Unverified))
	}
	if !strings.Contains(d.Unverified[0].Detail, "0 of 3") {
		t.Errorf("detail = %q, want the counts named", d.Unverified[0].Detail)
	}

	// Sign it off, and the same comparison reports the drift closing.
	f.commitContents("Tick it\n\nSpec: tb-x", now.AddDate(0, 0, -1),
		map[string]string{specFile("tb-x"): specWith("tb-x", "Unread when it landed", "x", "x", "x")})
	after, err := SinceDiff(f.dir, mark)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Unverified) != 0 {
		t.Errorf("signed-off work is not unverified: %+v", after.Unverified)
	}
	if changeIDs(after.Ticked) != "tb-x" {
		t.Errorf("ticked = %q, want tb-x", changeIDs(after.Ticked))
	}
}
