package report

import (
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

func sampleSummary() *audit.Summary {
	return &audit.Summary{
		Scope:    "Sprint s12, 27 July – 7 August",
		Sprint:   "s12",
		Headline: "2 stories delivered in sprint s12 — 13 of 21 points, 1 story paused.",
		Delivered: []audit.SummaryItem{
			{Title: "Usage metering pipeline", ID: "tb-1a2b", Points: 8},
			{Title: "Invoice PDF export", ID: "tb-2c3d", Points: 5},
		},
		InFlight: []audit.SummaryItem{{Title: "Stripe webhook retries", ID: "tb-3e4f", Points: 5}},
		Paused: []audit.SummaryItem{
			{Title: "Billing settings page", ID: "tb-4a5b", Points: 3, Reason: "waiting on the new billing design"},
		},
		NotStarted:      []audit.SummaryItem{{Title: "Dunning emails", ID: "tb-6e7f"}},
		PointsDelivered: 13,
		PointsTotal:     21,
		Unestimated:     1,
	}
}

func render(t *testing.T, s *audit.Summary, withIDs bool) string {
	t.Helper()
	var b strings.Builder
	if err := Summary(&b, s, withIDs); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Identifiers in the default output are how a report stops being readable.
func TestSummaryHidesIdentifiersUnlessAsked(t *testing.T) {
	out := render(t, sampleSummary(), false)
	for _, id := range []string{"tb-1a2b", "tb-4a5b", "tb-6e7f"} {
		if strings.Contains(out, id) {
			t.Errorf("default output leaks %q:\n%s", id, out)
		}
	}
	for _, want := range []string{
		"Usage metering pipeline", "8 points", "Billing settings page",
		"waiting on the new billing design", "Not started yet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSummaryAddsIdentifiersOnRequest(t *testing.T) {
	out := render(t, sampleSummary(), true)
	for _, id := range []string{"tb-1a2b", "tb-4a5b", "tb-6e7f"} {
		if !strings.Contains(out, id) {
			t.Errorf("--ids output missing %q:\n%s", id, out)
		}
	}
}

// The section headings are the vocabulary contract in visible form.
func TestSummaryUsesPlainHeadingsNotStatusNames(t *testing.T) {
	out := render(t, sampleSummary(), false)
	for _, heading := range []string{"## Delivered", "## Being worked on", "## Paused", "## Not started yet"} {
		if !strings.Contains(out, heading) {
			t.Errorf("missing heading %q:\n%s", heading, out)
		}
	}
	for _, banned := range []string{"## Done", "## In progress", "## Stalled", "## Planned"} {
		if strings.Contains(out, banned) {
			t.Errorf("derived-status heading %q reached a business report:\n%s", banned, out)
		}
	}
}

// An unestimated story is not a zero-point story, and the footnote has to
// agree with itself grammatically at n=1.
func TestUnestimatedFootnoteReadsCorrectlyForOne(t *testing.T) {
	out := render(t, sampleSummary(), false)
	if !strings.Contains(out, "1 open story carries no estimate") {
		t.Errorf("footnote misreads at n=1:\n%s", out)
	}
	s := sampleSummary()
	s.Unestimated = 3
	if out := render(t, s, false); !strings.Contains(out, "3 open stories carry no estimate") {
		t.Errorf("footnote misreads at n=3:\n%s", out)
	}
}

func TestEmptySectionsAreAbsentNotEmpty(t *testing.T) {
	s := sampleSummary()
	s.Paused = nil
	if out := render(t, s, false); strings.Contains(out, "## Paused") {
		t.Errorf("an empty section was rendered:\n%s", out)
	}
}
