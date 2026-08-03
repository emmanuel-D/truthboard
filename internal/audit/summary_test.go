package audit

import (
	"strings"
	"testing"
	"time"
)

// The vocabulary rule is the substance of this feature, not decoration: a
// summary that says "stalled" or names a branch has failed at its one job.
var jargon = []string{
	"done", "in-progress", "in progress", "stalled", "planned", "regressed",
	"in-review", "unmerged", "shadow work", "stale promise", "commit", "branch",
	"feature/", "tb-",
}

func assertNoJargon(t *testing.T, where, text string) {
	t.Helper()
	low := strings.ToLower(text)
	for _, word := range jargon {
		if strings.Contains(low, word) {
			t.Errorf("%s leaks %q into a summary meant for someone who does not read git:\n%s", where, word, text)
		}
	}
}

func summaryFixture() *Result {
	return &Result{
		DigestDays:  14,
		Integration: "origin/main",
		GeneratedAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		Shipped: []ShippedSpec{
			{ID: "tb-0001", Title: "Usage metering pipeline", Date: "2026-07-30"},
		},
		Units: []Unit{
			{Name: "feature/tb-0004-settings", LastCommit: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)},
		},
		Specs: []SpecStatus{
			{ID: "tb-0001", Title: "Usage metering pipeline", Sprint: "s12", Status: Done, Points: 8},
			{ID: "tb-0002", Title: "Stripe webhook retries", Sprint: "s12", Status: InProgress, Points: 5},
			{ID: "tb-0003", Title: "Proration on plan change", Sprint: "s12", Status: Planned, Points: 8},
			{ID: "tb-0004", Title: "Billing settings page", Sprint: "s12", Status: Stalled, Points: 3,
				Branches: []string{"feature/tb-0004-settings"}},
		},
		Sprints: []SprintRollup{{Name: "s12", Start: "2026-07-27", End: "2026-08-07"}},
	}
}

func TestSummarySortsWorkIntoPlainLanguageBuckets(t *testing.T) {
	s, err := summaryFixture().Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Delivered) != 1 || s.Delivered[0].Title != "Usage metering pipeline" {
		t.Errorf("delivered = %+v, want the landed story", s.Delivered)
	}
	if len(s.InFlight) != 1 || len(s.Paused) != 1 || len(s.NotStarted) != 1 {
		t.Errorf("buckets: inflight=%d paused=%d notstarted=%d, want 1/1/1",
			len(s.InFlight), len(s.Paused), len(s.NotStarted))
	}
	if s.PointsDelivered != 8 || s.PointsTotal != 24 {
		t.Errorf("points = %d of %d, want 8 of 24", s.PointsDelivered, s.PointsTotal)
	}
	if !strings.Contains(s.Headline, "8 of 24 points") {
		t.Errorf("headline = %q, want the commitment stated", s.Headline)
	}
	assertNoJargon(t, "headline", s.Headline)
	assertNoJargon(t, "scope", s.Scope)
}

// Over a rolling window there is no commitment, so a denominator would be
// the whole backlog — a number that says nothing about the fortnight.
func TestSummaryDoesNotInventADenominatorForTheDigestWindow(t *testing.T) {
	s, err := summaryFixture().Summarise("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(s.Headline, " of ") {
		t.Errorf("headline = %q, want no commitment ratio outside a sprint", s.Headline)
	}
	if !strings.Contains(s.Headline, "8 points") {
		t.Errorf("headline = %q, want the delivered points stated plainly", s.Headline)
	}
}

// Only work that landed *inside* the window is news; a story delivered
// last quarter would inflate the number.
func TestSummaryCountsOnlyWorkLandedInTheWindow(t *testing.T) {
	res := summaryFixture()
	res.Specs = append(res.Specs, SpecStatus{ID: "tb-0009", Title: "Ancient history", Status: Done, Points: 13})
	s, err := res.Summarise("")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range s.Delivered {
		if d.Title == "Ancient history" {
			t.Error("a story that landed before the window was counted as delivered in it")
		}
	}
	if s.PointsDelivered != 8 {
		t.Errorf("points delivered = %d, want 8", s.PointsDelivered)
	}
}

func TestPauseReasonPrefersTheHumanNote(t *testing.T) {
	res := summaryFixture()
	res.Specs[3].Hold = "waiting on the new billing design from Maya"
	s, err := res.Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Paused[0].Reason; got != "waiting on the new billing design from Maya" {
		t.Errorf("reason = %q, want the human note to outrank the derived facts", got)
	}
}

// A note git has already disproved must never be laundered into a report
// for people who cannot check it.
func TestContradictedHoldIsNeverPresentedAsALiveReason(t *testing.T) {
	res := summaryFixture()
	res.Specs[3].Hold = "waiting on the vendor"
	res.Specs[3].HoldContradicted = "work is moving"
	s, err := res.Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	got := s.Paused[0].Reason
	if strings.Contains(got, "vendor") {
		t.Errorf("reason = %q, want the contradicted note withheld", got)
	}
	if got == "" {
		t.Error("a paused story lost its reason entirely; say what is known instead")
	}
}

func TestPauseReasonNamesTheBlockingStoryNotItsID(t *testing.T) {
	res := summaryFixture()
	res.Specs[3].Waiting = []string{"tb-0003"}
	s, err := res.Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	got := s.Paused[0].Reason
	if !strings.Contains(got, "Proration on plan change") {
		t.Errorf("reason = %q, want the blocking story named in words", got)
	}
	assertNoJargon(t, "pause reason", got)
}

// With no note and no dependency, the only honest answer is how long it
// has been quiet.
func TestPauseReasonFallsBackToHowLongItHasBeenQuiet(t *testing.T) {
	s, err := summaryFixture().Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Paused[0].Reason; !strings.Contains(got, "13 days") {
		t.Errorf("reason = %q, want the quiet period stated", got)
	}
}

// A planned story someone deliberately parked is paused, not merely
// unstarted — that distinction is what makes the summary actionable.
func TestHeldPlannedWorkReadsAsPausedNotUnstarted(t *testing.T) {
	res := summaryFixture()
	res.Specs[2].Hold = "deprioritised until the tax work lands"
	s, err := res.Summarise("s12")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range s.NotStarted {
		if it.Title == "Proration on plan change" {
			t.Error("a deliberately parked story was filed as merely not started")
		}
	}
	if len(s.Paused) != 2 {
		t.Errorf("paused = %d, want the stalled story and the parked one", len(s.Paused))
	}
}

func TestSummaryFailsWhenThereIsNothingToSay(t *testing.T) {
	if _, err := (&Result{DigestDays: 14}).Summarise(""); err == nil {
		t.Error("empty summary should fail loudly rather than render an empty report")
	}
}

func TestAuditAttachesSummaryWithoutBeingAsked(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	writeSpecSprint(t, f.dir, "tb-s9a1", "A story nobody started", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary == nil {
		t.Fatal("audit produced no summary; the board and --format json need it with no API key")
	}
	if len(res.Summary.NotStarted) != 1 {
		t.Errorf("not started = %d, want 1", len(res.Summary.NotStarted))
	}
	assertNoJargon(t, "attached summary headline", res.Summary.Headline)
}
