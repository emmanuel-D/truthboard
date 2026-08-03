package report

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

// `ansi` is already a status→colour map in this package.
var ansiCodes = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func sprintResult() *audit.Result {
	return &audit.Result{
		Repo: "/tmp/x", Integration: "main", ElectedVia: "test",
		GeneratedAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		Sprints: []audit.SprintRollup{
			{
				Name: "s12", Done: 2, Total: 4,
				PointsDone: 13, PointsTotal: 21, Unestimated: 1,
				Start: "2026-07-27", End: "2026-08-07", State: "active", DaysLeft: 4,
				Open: []audit.SprintOpen{{ID: "tb-3e4f", Title: "Stripe webhook retries", Status: audit.InProgress}},
			},
			{Name: "s9", Done: 3, Total: 3}, // no estimates, no dates
		},
	}
}

func renderBoth(t *testing.T, res *audit.Result) (term, md string) {
	t.Helper()
	var tb, mb strings.Builder
	if err := Terminal(&tb, res, false); err != nil {
		t.Fatal(err)
	}
	if err := Markdown(&mb, res); err != nil {
		t.Fatal(err)
	}
	return ansiCodes.ReplaceAllString(tb.String(), ""), mb.String()
}

// The bug: Terminal called sprintPoints and sprintWindow, Markdown did not,
// so the format people paste into a status document was the one missing the
// numbers. Both renderers must state the same facts about a sprint.
func TestBothRenderersStateTheSameSprintFacts(t *testing.T) {
	term, md := renderBoth(t, sprintResult())
	for _, fact := range []string{
		"2/4 done",
		"13/21 pts",
		"(1 unestimated)",
		"2026-07-27 → 2026-08-07",
		"active",
		"4d left",
	} {
		if !strings.Contains(term, fact) {
			t.Errorf("terminal is missing %q:\n%s", fact, term)
		}
		if !strings.Contains(md, fact) {
			t.Errorf("markdown is missing %q — the paste-able format must not drop it:\n%s", fact, md)
		}
	}
}

// Stated as one assertion over the shared builder, so a third surface that
// composes from sprintFacts inherits the agreement rather than re-deriving
// it — and a fourth that does not will fail this test the day it lands.
func TestSprintFactsIsWhatBothRenderersEmit(t *testing.T) {
	res := sprintResult()
	term, md := renderBoth(t, res)
	facts := sprintFacts(res.Sprints[0])
	if facts != "2/4 done · 13/21 pts (1 unestimated) · 2026-07-27 → 2026-08-07 · active, 4d left" {
		t.Fatalf("sprintFacts = %q, not the agreed sentence", facts)
	}
	if !strings.Contains(md, facts) {
		t.Errorf("markdown does not emit sprintFacts verbatim:\n%s", md)
	}
	// The terminal keeps its own hierarchy — bright count, dim detail — so
	// it emits the two halves rather than the joined string.
	if !strings.Contains(term, "2/4 done"+sprintDetail(res.Sprints[0])) {
		t.Errorf("terminal does not emit the same facts:\n%s", term)
	}
}

// A repo that estimates nothing and dates nothing must look exactly as it
// did before, with no stray separators left behind.
func TestUndatedUnestimatedSprintsDegradeIdentically(t *testing.T) {
	term, md := renderBoth(t, sprintResult())
	if !strings.Contains(md, "- **s9** — 3/3 done · open:") && !strings.Contains(md, "- **s9** — 3/3 done\n") {
		t.Errorf("markdown left a dangling separator on a bare sprint:\n%s", md)
	}
	if !strings.Contains(term, "s9  3/3 done\n") {
		t.Errorf("terminal changed for a bare sprint:\n%s", term)
	}
	if got := sprintFacts(audit.SprintRollup{Name: "s9", Done: 3, Total: 3}); got != "3/3 done" {
		t.Errorf("bare sprint facts = %q, want just the count", got)
	}
}
