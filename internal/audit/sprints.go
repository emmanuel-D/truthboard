package audit

import (
	"sort"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// SprintRollup groups spec statuses by their sprint slug. A sprint has no
// status of its own — done/total is arithmetic over the same derived
// statuses shown everywhere else, and a sprint is "finished" exactly when
// its stories are. Open stories carry their status so the rollup answers
// "what rolls over" without inventing a sprint clock.
//
// When a sprint intent file (.truthboard/sprints/<slug>.md) declares
// start/end dates, the rollup also carries a *derived* calendar state —
// future, active, or completed, computed from today's date against the
// window. There is still nothing to type: a date window is intent, the
// state falls out of it.
type SprintRollup struct {
	Name     string       `json:"name"`
	Done     int          `json:"done"`
	Total    int          `json:"total"`
	Open     []SprintOpen `json:"open,omitempty"`      // everything not done, backlog order
	Start    string       `json:"start,omitempty"`     // from the sprint intent file
	End      string       `json:"end,omitempty"`       // inclusive
	State    string       `json:"state,omitempty"`     // future | active | completed — derived from dates
	DaysLeft int          `json:"days_left,omitempty"` // active sprints: days until end, 0 = ends today
}

type SprintOpen struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status Status `json:"status"`
}

// rollupSprints derives the per-sprint view from res.Specs (already in
// backlog order) plus any sprint intent files. Specs without a sprint
// don't appear — sprints are opt-in, and a repo that never uses them sees
// nothing new. A dated sprint file with no stories yet still appears, so a
// planned window is visible before stories are pulled into it.
func rollupSprints(res *Result, intents []spec.SprintIntent, now time.Time) {
	byName := map[string]*SprintRollup{}
	for _, s := range res.Specs {
		if s.Sprint == "" {
			continue
		}
		r := byName[s.Sprint]
		if r == nil {
			r = &SprintRollup{Name: s.Sprint}
			byName[s.Sprint] = r
		}
		r.Total++
		if s.Status == Done {
			r.Done++
		} else {
			r.Open = append(r.Open, SprintOpen{ID: s.ID, Title: s.Title, Status: s.Status})
		}
	}
	for _, in := range intents {
		r := byName[in.Slug]
		if r == nil {
			r = &SprintRollup{Name: in.Slug}
			byName[in.Slug] = r
		}
		r.Start, r.End = in.Start, in.End
		r.State, r.DaysLeft = sprintState(in, now)
	}
	if len(byName) == 0 {
		return
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	// Newest sprint first under the usual naming schemes (s12, 2026-29):
	// longer slugs are later, ties break lexicographically descending.
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] > names[j]
	})
	for _, n := range names {
		res.Sprints = append(res.Sprints, *byName[n])
	}
}

// sprintState derives future/active/completed from the date window. Both
// dates are needed for a state — a half-specified window stays stateless
// rather than guessing. The end date is inclusive: a sprint ending today
// is still active with 0 days left.
func sprintState(in spec.SprintIntent, now time.Time) (string, int) {
	if in.Start == "" || in.End == "" {
		return "", 0
	}
	start, err1 := time.Parse(spec.DateLayout, in.Start)
	end, err2 := time.Parse(spec.DateLayout, in.End)
	if err1 != nil || err2 != nil {
		return "", 0
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch {
	case today.Before(start):
		return "future", 0
	case today.After(end):
		return "completed", 0
	default:
		return "active", int(end.Sub(today).Hours() / 24)
	}
}
