package llm

import (
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

func flowFixture(stories int) *audit.Result {
	res := planFixture()
	res.Flow = &audit.Flow{
		From: "2026-05-16", To: "2026-08-14", Days: 90,
		Cycle:   audit.Stat{Stories: stories, MedianHours: 52, P85Hours: 190, MinHours: 3, MaxHours: 240},
		Lead:    audit.Stat{Stories: stories, MedianHours: 96},
		OpenNow: 2,
		Sprints: []audit.Bucket{
			{Label: "s12", Stories: 6, Points: 13, Unestimated: 1},
			{Label: "s11", Stories: 4, Points: 9, Unestimated: 1},
		},
	}
	// s12 is the sprint running now; s11 finished.
	res.Sprints = append(res.Sprints, audit.SprintRollup{
		Name: "s11", Start: "2026-07-13", End: "2026-07-24", State: "completed"})
	for i := 0; i < stories; i++ {
		res.Flow.Stories = append(res.Flow.Stories, audit.StoryFlow{ID: "tb-x", CycleHours: 52})
	}
	return res
}

// TestFlowFactsCarryTheirSampleSize covers the rule that makes derived
// history safe to narrate: every figure travels with the window and the
// number of stories behind it.
func TestFlowFactsCarryTheirSampleSize(t *testing.T) {
	facts := flowFacts(flowFixture(20))
	for _, want := range []string{
		"2026-05-16 → 2026-08-14",
		"2.2d median",      // cycle
		"7.9d at the 85th", // p85
		"over 20 story(ies)",
		"4.0d median", // lead
		"1.6 story(ies) landed per week",
		"2 in flight",
		"sprint s11 landed 4 story(ies), 9 points",
		"(1 unestimated, excluded from the sum)",
	} {
		if !strings.Contains(facts, want) {
			t.Errorf("flow facts missing %q:\n%s", want, facts)
		}
	}
	// s12 is still running: a sprint that has not finished has not landed
	// everything it will, and offering it as history invites a comparison
	// between a whole sprint and a partial one.
	if strings.Contains(facts, "sprint s12 landed") {
		t.Errorf("an unfinished sprint must not be offered as history:\n%s", facts)
	}
	if strings.Contains(facts, "too few") {
		t.Errorf("20 stories is a sample, not an anecdote:\n%s", facts)
	}
}

// TestThinHistoryIsNotOfferedAsATrend is the honesty guard the planning
// rollup has always applied to its single-sprint load reference, now applied
// to timed history: the figures are still reported, and the narrator is told
// in the facts themselves not to draw a line through them.
func TestThinHistoryIsNotOfferedAsATrend(t *testing.T) {
	facts := flowFacts(flowFixture(3))
	if !strings.Contains(facts, "over 3 story(ies)") {
		t.Errorf("the figures must still be reported:\n%s", facts)
	}
	if !strings.Contains(facts, "too few to describe a trend") {
		t.Errorf("a three-story sample must be named as too thin:\n%s", facts)
	}
}

// TestNothingLandedIsSaidPlainly guards the other end: an empty window must
// read as "nothing landed", never as a pace of zero.
func TestNothingLandedIsSaidPlainly(t *testing.T) {
	res := planFixture()
	res.Flow = &audit.Flow{From: "2026-05-16", To: "2026-08-14", Days: 90, OpenNow: 4}
	facts := flowFacts(res)
	if !strings.Contains(facts, "nothing landed in the last 90 days") {
		t.Errorf("an empty window must say so:\n%s", facts)
	}
	if !strings.Contains(facts, "do not describe a pace") {
		t.Errorf("the narrator must be told not to invent a pace:\n%s", facts)
	}
	if strings.Contains(facts, "per week") {
		t.Errorf("no rate may be quoted with nothing behind it:\n%s", facts)
	}
}

// TestPlanAndReviewBothCarryFlow keeps the two narrators reading from the
// same facts — a review that knows the cycle time and a plan that does not
// would be two answers to one question.
func TestPlanAndReviewBothCarryFlow(t *testing.T) {
	res := flowFixture(20)
	plan, err := planFacts(res, "")
	if err != nil {
		t.Fatal(err)
	}
	review, err := reviewFacts(res, "")
	if err != nil {
		t.Fatal(err)
	}
	for name, facts := range map[string]string{"plan": plan, "review": review} {
		if !strings.Contains(facts, "cycle time") {
			t.Errorf("%s facts carry no flow history:\n%s", name, facts)
		}
	}
}
