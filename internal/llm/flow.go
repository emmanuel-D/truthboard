package llm

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

// trendFloor is how many timed stories it takes before flow history may be
// described as a shape rather than a list of facts. Below it the numbers
// are still true and still stated — what changes is that the narrator is
// told, in the facts themselves, not to draw a line through them.
const trendFloor = 8

// flowFacts renders the timed history for a narrator. It exists because
// `plan` and `review` used to have no history to work from at all: the only
// load figure available was a single prior sprint, which the planning
// rollup correctly refused to call a velocity. Cycle time and per-week
// throughput are real history, derived from merges — but a real history of
// three stories is still an anecdote, and the sample size travels with
// every figure so the model cannot quietly promote one into a trend.
func flowFacts(res *audit.Result) string {
	f := res.Flow
	if !f.Measured() {
		return ""
	}
	var b strings.Builder

	if f.Cycle.Empty() {
		fmt.Fprintf(&b, "Flow: nothing landed in the last %d days", f.Days)
		if f.OpenNow > 0 {
			fmt.Fprintf(&b, "; %d story(ies) are in flight", f.OpenNow)
		}
		b.WriteString(". State this plainly and do not describe a pace.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "Flow (timed from commits, %s → %s): cycle time — first work commit to the merge that landed it — is %s median, %s at the 85th percentile, over %d story(ies).\n",
		f.From, f.To, audit.Duration(f.Cycle.MedianHours), audit.Duration(f.Cycle.P85Hours), f.Cycle.Stories)
	if !f.Lead.Empty() {
		fmt.Fprintf(&b, "Flow: lead time — story filed to landed — is %s median over %d story(ies).\n",
			audit.Duration(f.Lead.MedianHours), f.Lead.Stories)
	}
	fmt.Fprintf(&b, "Flow: %.1f story(ies) landed per week across the window; %d in flight right now.\n", f.PerWeek(), f.OpenNow)

	// Per-sprint throughput is the several data points the single-sprint
	// load reference never had. Only completed sprints are offered: a sprint
	// still running has landed only part of what it will.
	shown := 0
	for _, sp := range f.Sprints {
		if shown == 6 {
			break
		}
		state := sprintState(res, sp.Label)
		if state != "completed" {
			continue
		}
		fmt.Fprintf(&b, "Flow: sprint %s landed %d story(ies)", sp.Label, sp.Stories)
		if sp.Points > 0 {
			fmt.Fprintf(&b, ", %d points", sp.Points)
		}
		if sp.Unestimated > 0 {
			fmt.Fprintf(&b, " (%d unestimated, excluded from the sum)", sp.Unestimated)
		}
		b.WriteString("\n")
		shown++
	}

	if f.Cycle.Stories < trendFloor {
		fmt.Fprintf(&b, "Flow caveat: %d timed story(ies) is too few to describe a trend or a direction. Report the figures with their sample size and say no more about them.\n", f.Cycle.Stories)
	}
	if n := len(f.Unmeasurable); n > 0 {
		fmt.Fprintf(&b, "Flow caveat: %d landed story(ies) could not be timed at all and are excluded from every figure above.\n", n)
	}
	return b.String()
}

func sprintState(res *audit.Result, name string) string {
	for _, sp := range res.Sprints {
		if sp.Name == name {
			return sp.State
		}
	}
	return ""
}
