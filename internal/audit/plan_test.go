package audit

import (
	"testing"
	"time"
)

// planResult builds a Result directly: PlanFor is pure arithmetic over
// derived statuses, so it can be exercised without a git fixture. The
// end-to-end wiring gets its own test below.
func planResult(sprints []SprintRollup, specs []SpecStatus) *Result {
	return &Result{Sprints: sprints, Specs: specs}
}

func ids(stories []PlanStory) []string {
	out := make([]string, 0, len(stories))
	for _, s := range stories {
		out = append(out, s.ID)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPlanSplitsRolloverCommittedReadyBlocked(t *testing.T) {
	res := planResult(
		[]SprintRollup{
			{Name: "s13", Start: "2026-08-10", End: "2026-08-21", State: "future"},
			{Name: "s12", Start: "2026-07-27", End: "2026-08-07", State: "active", PointsDone: 13},
		},
		[]SpecStatus{
			{ID: "tb-0001", Title: "Landed last sprint", Sprint: "s12", Status: Done, Points: 8},
			{ID: "tb-0002", Title: "Did not land", Sprint: "s12", Status: InProgress, Points: 5},
			{ID: "tb-0003", Title: "Already pulled in", Sprint: "s13", Status: Planned, Points: 3},
			{ID: "tb-0004", Title: "Pulled in, no estimate", Sprint: "s13", Status: Planned},
			{ID: "tb-0005", Title: "Free candidate", Status: Planned, Points: 2},
			{ID: "tb-0006", Title: "Waiting on work", Status: Planned, Points: 1, Waiting: []string{"tb-0002"}},
		},
	)

	p, err := res.PlanFor("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Sprint != "s13" {
		t.Fatalf("target sprint = %q, want the next future one (s13)", p.Sprint)
	}
	if p.From != "s12" {
		t.Fatalf("rollover source = %q, want the active sprint (s12)", p.From)
	}
	if got := ids(p.Rollover); !equal(got, []string{"tb-0002"}) {
		t.Errorf("rollover = %v, want only the unlanded s12 story", got)
	}
	if got := ids(p.Committed); !equal(got, []string{"tb-0003", "tb-0004"}) {
		t.Errorf("committed = %v, want the two stories already carrying s13", got)
	}
	if got := ids(p.Ready); !equal(got, []string{"tb-0005"}) {
		t.Errorf("ready = %v, want the unblocked unsprinted story", got)
	}
	if got := ids(p.Blocked); !equal(got, []string{"tb-0006"}) {
		t.Errorf("blocked = %v, want the story with an unmet need", got)
	}
	if p.Blocked[0].Waiting[0] != "tb-0002" {
		t.Errorf("blocked story names %v as blocking, want tb-0002", p.Blocked[0].Waiting)
	}
}

func TestPlanCountsPointsWithoutTreatingUnestimatedAsZero(t *testing.T) {
	res := planResult(
		[]SprintRollup{
			{Name: "s13", Start: "2026-08-10", End: "2026-08-21", State: "future"},
			{Name: "s12", Start: "2026-07-27", End: "2026-08-07", State: "active", PointsDone: 13},
		},
		[]SpecStatus{
			{ID: "tb-0003", Sprint: "s13", Status: Planned, Points: 3},
			{ID: "tb-0004", Sprint: "s13", Status: Planned},
			{ID: "tb-0002", Sprint: "s12", Status: Stalled, Points: 5},
		},
	)

	p, err := res.PlanFor("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Points != 3 {
		t.Errorf("committed points = %d, want 3 (the unestimated story is excluded, not zero)", p.Points)
	}
	if p.Unestimated != 1 {
		t.Errorf("unestimated = %d, want 1", p.Unestimated)
	}
	if p.RolloverPoints != 5 {
		t.Errorf("rollover points = %d, want 5", p.RolloverPoints)
	}
	if p.Reference != 13 || p.ReferenceSprint != "s12" {
		t.Errorf("load reference = %d from %q, want 13 from s12", p.Reference, p.ReferenceSprint)
	}
}

func TestPlanTargetsNamedSprintThatDoesNotExistYet(t *testing.T) {
	// Planning into a slug is how the slug comes to exist — an unknown
	// name is a target, not an error, as long as there is something to plan.
	res := planResult(
		[]SprintRollup{{Name: "s12", Start: "2026-07-27", End: "2026-08-07", State: "active", PointsDone: 8}},
		[]SpecStatus{{ID: "tb-0005", Status: Planned, Points: 2}},
	)

	p, err := res.PlanFor("s99")
	if err != nil {
		t.Fatal(err)
	}
	if p.Sprint != "s99" {
		t.Errorf("target = %q, want the named slug s99", p.Sprint)
	}
	if p.Start != "" || p.State != "" {
		t.Errorf("undated target carries window %q/%q, want none", p.Start, p.State)
	}
	if got := ids(p.Ready); !equal(got, []string{"tb-0005"}) {
		t.Errorf("ready = %v, want tb-0005", got)
	}
}

func TestPlanWithoutFutureSprintReportsCandidatesAndNoTarget(t *testing.T) {
	res := planResult(
		[]SprintRollup{{Name: "s12", Start: "2026-07-27", End: "2026-08-07", State: "active", PointsDone: 8}},
		[]SpecStatus{
			{ID: "tb-0002", Sprint: "s12", Status: InProgress, Points: 5},
			{ID: "tb-0005", Status: Planned, Points: 2},
		},
	)

	p, err := res.PlanFor("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Sprint != "" {
		t.Errorf("target = %q, want none — no dated sprint is waiting to start", p.Sprint)
	}
	if len(p.Rollover) != 1 || len(p.Ready) != 1 {
		t.Errorf("rollover=%d ready=%d, want the plan to still report both", len(p.Rollover), len(p.Ready))
	}
}

func TestPlanFailsLoudlyWithNothingToPlan(t *testing.T) {
	res := planResult(nil, []SpecStatus{{ID: "tb-0001", Status: Done}})
	if _, err := res.PlanFor(""); err == nil {
		t.Fatal("PlanFor succeeded with no sprint, no rollover and no candidates; want a loud error")
	}
}

func TestPlanFallsBackToNewestSlugWhenSprintsAreUndated(t *testing.T) {
	// Teams using sprint slugs without intent files still get a rollover
	// source: res.Sprints is ordered newest first.
	res := planResult(
		[]SprintRollup{{Name: "s12", PointsDone: 6}, {Name: "s9", PointsDone: 4}},
		[]SpecStatus{{ID: "tb-0002", Sprint: "s12", Status: InProgress, Points: 5}},
	)

	p, err := res.PlanFor("s13")
	if err != nil {
		t.Fatal(err)
	}
	if p.From != "s12" || p.ReferenceSprint != "s12" {
		t.Errorf("rollover source = %q (ref %q), want the newest slug s12", p.From, p.ReferenceSprint)
	}
}

func TestPlanKeepsBacklogOrder(t *testing.T) {
	// res.Specs arrives sorted by priority then id; the plan lists must not
	// resort or interleave.
	res := planResult(
		[]SprintRollup{{Name: "s13", Start: "2026-08-10", End: "2026-08-21", State: "future"}},
		[]SpecStatus{
			{ID: "tb-000a", Status: Planned, Priority: 1},
			{ID: "tb-000b", Status: Planned, Priority: 2},
			{ID: "tb-000c", Status: Planned, Priority: 3},
		},
	)

	p, err := res.PlanFor("")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(p.Ready); !equal(got, []string{"tb-000a", "tb-000b", "tb-000c"}) {
		t.Errorf("ready = %v, want backlog order preserved", got)
	}
}

func TestPlanDaysCountsInclusiveWindow(t *testing.T) {
	p := &PlanRollup{Start: "2026-08-10", End: "2026-08-21"}
	if got := p.Days(); got != 12 {
		t.Errorf("Days() = %d, want 12 (end date inclusive)", got)
	}
	if got := (&PlanRollup{}).Days(); got != 0 {
		t.Errorf("Days() with no window = %d, want 0", got)
	}
}

func TestAuditAttachesPlanForJSONConsumers(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.git("checkout", "-b", "feature/tb-p1b-wip")
	f.commit("feat: wip\n\nSpec: tb-p1b", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	writeSpecSprint(t, f.dir, "tb-p1b", "Did not land", "s12")
	writeSpecSprint(t, f.dir, "tb-p1c", "Free candidate", "")
	writeSprintFile(t, f.dir, "s12", "2026-07-27", "2026-08-07")
	writeSprintFile(t, f.dir, "s13", "2026-08-10", "2026-08-21")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Plan == nil {
		t.Fatal("audit produced no plan; --format json consumers need it without an LLM")
	}
	if res.Plan.Sprint != "s13" {
		t.Errorf("plan target = %q, want s13", res.Plan.Sprint)
	}
	if res.Plan.From != "s12" {
		t.Errorf("plan rollover source = %q, want s12", res.Plan.From)
	}
	if got := ids(res.Plan.Rollover); !equal(got, []string{"tb-p1b"}) {
		t.Errorf("rollover = %v, want tb-p1b", got)
	}
	if got := ids(res.Plan.Ready); !equal(got, []string{"tb-p1c"}) {
		t.Errorf("ready = %v, want tb-p1c", got)
	}
}
