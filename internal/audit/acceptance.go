package audit

// A story can be done and unverified at the same time, and for a while
// Truthboard had no word for it. Git proved the commit landed, the card
// flipped, and the checklist underneath it stayed exactly as the PO wrote
// it — nobody could tell which criteria had been read back and which had
// merely been assumed. Landed stories kept arriving 0/3.
//
// The status is not the problem: the work did land, and derived status is
// the one thing here git owns outright. What was missing is that skipping
// the read-back cost nothing. Every other lapse in this repo — a promise
// gone stale, work with no story, a hold the evidence disproved — gets
// reported. This one didn't, so it kept happening.
//
// So it becomes drift, and only drift: a spec derived done whose criteria
// are not all ticked is listed beside the other lapses and changes no
// status anywhere. A done spec with no checklist at all is not drift —
// there is nothing to verify, and the specs written before anyone counted
// must not turn the board red.

import (
	"fmt"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// UnverifiedAcceptance is a landed promise nobody signed off: the work is
// done by git's account, and its acceptance criteria still are not ticked.
type UnverifiedAcceptance struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Done     int      `json:"done"`
	Total    int      `json:"total"`
	Unticked []string `json:"unticked,omitempty"` // the criteria still open, in order
}

// Summary reads as a sentence wherever the drift is printed.
func (u UnverifiedAcceptance) Summary() string {
	return fmt.Sprintf("%d of %d criteria ticked", u.Done, u.Total)
}

// SignoffReminder is the line every "here is your next task" surface prints
// before the task: the work you already landed still has promises nobody
// read back. One sentence, the verb to fix it, and no way to mistake it for
// a status — the story is done either way.
//
// Empty when there is nothing to remind anyone of, so callers can print it
// unconditionally.
func SignoffReminder(us []UnverifiedAcceptance) string {
	if len(us) == 0 {
		return ""
	}
	u := us[0]
	s := fmt.Sprintf("Before starting: %s (%s) landed with %s. Verify what is true and tick it: truthboard check %s all — or check_acceptance over MCP.",
		u.ID, u.Title, u.Summary(), u.ID)
	if len(us) > 1 {
		s += fmt.Sprintf(" (%d more landed stories are unverified — truthboard audit lists them.)", len(us)-1)
	}
	return s
}

// deriveUnverifiedAcceptance collects done specs whose checklist is not
// fully ticked. Runs after statuses are final; it reads them, never writes
// one. Regressed specs are left out on purpose — their story is the
// regression, and asking for a sign-off on work that came undone would be
// noise on top of an alarm.
func deriveUnverifiedAcceptance(res *Result, specs []spec.Spec) {
	bodies := make(map[string]string, len(specs))
	for i := range specs {
		bodies[specs[i].ID] = specs[i].Body
	}
	for i := range res.Specs {
		ss := &res.Specs[i]
		if ss.Status != Done || ss.AcceptanceTotal == 0 || ss.AcceptanceDone == ss.AcceptanceTotal {
			continue
		}
		u := UnverifiedAcceptance{
			ID: ss.ID, Title: ss.Title,
			Done: ss.AcceptanceDone, Total: ss.AcceptanceTotal,
		}
		for _, c := range spec.Criteria(bodies[ss.ID]) {
			if !c.Checked {
				u.Unticked = append(u.Unticked, c.Text)
			}
		}
		res.Drift.UnverifiedAcceptance = append(res.Drift.UnverifiedAcceptance, u)
	}
}

// pruneUnverifiedAcceptance drops entries whose spec has stopped being done
// since the list was built — forge enrichment reads CI, and red CI regresses
// a landing. Work that came undone is not waiting on a sign-off.
func pruneUnverifiedAcceptance(res *Result) {
	if len(res.Drift.UnverifiedAcceptance) == 0 {
		return
	}
	done := map[string]bool{}
	for _, ss := range res.Specs {
		done[ss.ID] = ss.Status == Done
	}
	kept := res.Drift.UnverifiedAcceptance[:0]
	for _, u := range res.Drift.UnverifiedAcceptance {
		if done[u.ID] {
			kept = append(kept, u)
		}
	}
	res.Drift.UnverifiedAcceptance = kept
}
