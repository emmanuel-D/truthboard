package llm

import (
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

func planFixture() *audit.Result {
	return &audit.Result{
		Integration: "main", DigestDays: 14,
		Specs: []audit.SpecStatus{
			{ID: "tb-0002", Title: "Did not land", Sprint: "s12", Status: audit.InProgress, Points: 5},
			{ID: "tb-0003", Title: "Already pulled in", Sprint: "s13", Status: audit.Planned, Points: 3},
			{ID: "tb-0004", Title: "No estimate", Sprint: "s13", Status: audit.Planned},
			{ID: "tb-0005", Title: "Free candidate", Epic: "po-experience", Status: audit.Planned, Points: 2},
			{ID: "tb-0006", Title: "Waiting story", Status: audit.Planned, Points: 1, Waiting: []string{"tb-0002"}},
		},
		Sprints: []audit.SprintRollup{
			{Name: "s13", Start: "2026-08-10", End: "2026-08-21", State: "future"},
			{Name: "s12", Start: "2026-07-27", End: "2026-08-07", State: "active", PointsDone: 13},
		},
	}
}

func TestPlanFactsAreDerivedNotInvented(t *testing.T) {
	facts, err := planFacts(planFixture(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sprint s13",              // the auto-chosen target
		"2026-08-10 → 2026-08-21", // its window
		"12 days",
		"Rolling over from s12",
		"Rollover: tb-0002",
		"Committed: tb-0003",
		"3 points",
		"1 unestimated",
		"Ready to pull in: tb-0005",
		"[po-experience]",
		"Blocked: tb-0006",
		"waiting on tb-0002",
	} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts missing %q:\n%s", want, facts)
		}
	}
}

func TestPlanFactsRefuseToImplyVelocity(t *testing.T) {
	facts, err := planFacts(planFixture(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts, "sprint s12 landed 13 points") {
		t.Errorf("facts drop the load reference:\n%s", facts)
	}
	if !strings.Contains(facts, "not a velocity") {
		t.Errorf("facts state a reference without disclaiming the trend:\n%s", facts)
	}
	if strings.Contains(strings.ToLower(facts), "average") {
		t.Errorf("facts imply an average over sprints that do not exist:\n%s", facts)
	}
}

func TestPlanFactsSayWhenThereIsNoTarget(t *testing.T) {
	res := planFixture()
	res.Sprints = res.Sprints[1:] // drop the future sprint; only the active one remains
	facts, err := planFacts(res, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts, "Planning target: none") {
		t.Errorf("facts invent a target where none is derivable:\n%s", facts)
	}
	if !strings.Contains(facts, "Rollover: tb-0002") {
		t.Errorf("facts drop rollover just because there is no target:\n%s", facts)
	}
}

func TestPlanFactsFailWithNothingToPlan(t *testing.T) {
	res := &audit.Result{Specs: []audit.SpecStatus{{ID: "tb-0001", Status: audit.Done}}}
	if _, err := planFacts(res, ""); err == nil {
		t.Error("empty plan should error, not narrate an empty block")
	}
}
