package audit

import (
	"fmt"
	"sort"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// PlanRollup is the sprint-start counterpart to SprintRollup: everything a
// planning meeting needs, derived from the same statuses everything else
// derives from. Nothing here is typed. The target sprint is a name, the
// candidates come from backlog order, readiness comes from the needs graph,
// and the only load figure that means anything — what the last sprint
// actually landed — is arithmetic over merges.
//
// Deliberately absent: a velocity trend. One prior sprint is one data
// point, and the rollup says which sprint it came from so nobody reads it
// as a line through history. Real history now exists next door — Flow times
// every landed story from its commits — but it stays next door: a plan
// reports what a team committed to and what one prior sprint landed, and
// projecting either from a median would be inventing the one number this
// tool exists to stop people inventing.
type PlanRollup struct {
	Sprint string `json:"sprint,omitempty"` // target slug; empty when no sprint is being planned into
	Start  string `json:"start,omitempty"`  // from the target's intent file, when it has one
	End    string `json:"end,omitempty"`
	State  string `json:"state,omitempty"` // future | active | completed — derived, as everywhere

	From     string      `json:"from,omitempty"`     // the sprint the rollover comes from
	Rollover []PlanStory `json:"rollover,omitempty"` // its stories that did not land

	Committed []PlanStory `json:"committed,omitempty"` // already carrying the target sprint, not done
	Ready     []PlanStory `json:"ready,omitempty"`     // unsprinted, nothing unmet in needs
	Blocked   []PlanStory `json:"blocked,omitempty"`   // unsprinted, waiting on ids named in Waiting

	Points         int `json:"points,omitempty"`          // sum over Committed
	Unestimated    int `json:"unestimated,omitempty"`     // committed stories with no estimate — not counted as zero
	RolloverPoints int `json:"rollover_points,omitempty"` // sum over Rollover, if all of it is pulled in

	Reference       int    `json:"reference_points,omitempty"` // points the reference sprint actually landed
	ReferenceSprint string `json:"reference_sprint,omitempty"`
}

// PlanStory is a candidate or rollover story: intent plus its derived
// status and the subset of its needs that are not done yet.
type PlanStory struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Status   Status   `json:"status"`
	Epic     string   `json:"epic,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Points   int      `json:"points,omitempty"`
	Waiting  []string `json:"waiting,omitempty"`
}

// rollupPlan attaches the plan for the automatically chosen target to the
// result, so `--format json` carries a planning summary without anyone
// asking for one — and without an LLM key. A repo with nothing to plan
// simply has no plan, which is not an error at audit time.
func rollupPlan(res *Result) {
	p, err := res.PlanFor("")
	if err != nil {
		return
	}
	res.Plan = p
}

// PlanFor builds the planning rollup for one target sprint. An empty slug
// means "choose the obvious one": the next dated sprint that has not
// started. Naming a slug that does not exist yet is legitimate — planning
// into a sprint is how it comes to exist — so an unknown name is a target,
// not an error.
func (r *Result) PlanFor(sprint string) (*PlanRollup, error) {
	p := &PlanRollup{Sprint: sprint}
	if p.Sprint == "" {
		p.Sprint = nextSprint(r.Sprints)
	}
	for _, sp := range r.Sprints {
		if sp.Name == p.Sprint {
			p.Start, p.End, p.State = sp.Start, sp.End, sp.State
			break
		}
	}

	if from := closingSprint(r.Sprints, p.Sprint); from != nil {
		p.From = from.Name
		p.ReferenceSprint = from.Name
		p.Reference = from.PointsDone
	}

	// res.Specs is already in backlog order (priority, then id), so every
	// list below inherits it without sorting again.
	for _, s := range r.Specs {
		if s.Status == Done {
			continue
		}
		story := PlanStory{
			ID: s.ID, Title: s.Title, Status: s.Status, Epic: s.Epic,
			Priority: s.Priority, Points: s.Points, Waiting: s.Waiting,
		}
		switch {
		case p.Sprint != "" && s.Sprint == p.Sprint:
			p.Committed = append(p.Committed, story)
			p.Points += s.Points
			if s.Points == 0 {
				p.Unestimated++
			}
		case p.From != "" && s.Sprint == p.From:
			p.Rollover = append(p.Rollover, story)
			p.RolloverPoints += s.Points
		case s.Sprint == "":
			if len(s.Waiting) > 0 {
				p.Blocked = append(p.Blocked, story)
			} else {
				p.Ready = append(p.Ready, story)
			}
		}
	}

	if p.Sprint == "" && len(p.Rollover) == 0 && len(p.Ready) == 0 && len(p.Blocked) == 0 {
		return nil, fmt.Errorf("nothing to plan — no sprint to plan into, nothing rolling over, and no open stories outside a sprint")
	}
	return p, nil
}

// nextSprint picks the target when none was named: the dated sprint that
// has not started yet, earliest first. Without one there is no obvious
// answer, and inventing a slug would be typing a status by another name —
// the plan reports rollover and candidates with no target instead.
func nextSprint(sprints []SprintRollup) string {
	var future []SprintRollup
	for _, sp := range sprints {
		if sp.State == "future" {
			future = append(future, sp)
		}
	}
	if len(future) == 0 {
		return ""
	}
	sort.Slice(future, func(i, j int) bool {
		if future[i].Start != future[j].Start {
			return future[i].Start < future[j].Start
		}
		return future[i].Name < future[j].Name
	})
	return future[0].Name
}

// closingSprint is the sprint whose work rolls over and whose landed points
// are the one honest reference for load: the sprint running now, else the
// dated one that ended most recently, else — for teams that use sprint
// slugs without dates — the newest slug that is not the target, since
// res.Sprints is already ordered newest first.
func closingSprint(sprints []SprintRollup, target string) *SprintRollup {
	var completed, undated *SprintRollup
	for i := range sprints {
		sp := &sprints[i]
		if sp.Name == target {
			continue
		}
		switch sp.State {
		case "active":
			return sp
		case "completed":
			if completed == nil || sp.End > completed.End {
				completed = sp
			}
		case "":
			if undated == nil {
				undated = sp
			}
		}
	}
	if completed != nil {
		return completed
	}
	return undated
}

// Days reports how many days the target sprint runs, for surfaces that
// want the window as a length rather than two dates. Zero means the target
// has no usable window.
func (p *PlanRollup) Days() int {
	if p.Start == "" || p.End == "" {
		return 0
	}
	start, err1 := time.Parse(spec.DateLayout, p.Start)
	end, err2 := time.Parse(spec.DateLayout, p.End)
	if err1 != nil || err2 != nil || end.Before(start) {
		return 0
	}
	return int(end.Sub(start).Hours()/24) + 1 // end date is inclusive
}
