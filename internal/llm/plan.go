package llm

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

const planPrompt = `You are writing the opening summary for a sprint planning
meeting. Below are the derived facts from the team's git repository —
statuses come from commits and merges, not from anyone's claims. Write a
concise, honest planning summary in markdown: what rolls over and why it is
not done, what is already committed to the sprint, which candidates are ready
to pull in next and which are blocked and on what. If a load reference is
given, compare the commitment to it plainly and say it is a single prior
sprint, not a trend — never project a velocity. No invented work, no
encouragement padding, no facts beyond those given. 150-300 words.

%s`

// Plan narrates the sprint about to start from the derived Result. Like
// Review it is a writer, not a source: every number below is arithmetic the
// audit already did, and the model only puts it into sentences.
func Plan(p Provider, res *audit.Result, sprint string) (string, error) {
	facts, err := planFacts(res, sprint)
	if err != nil {
		return "", err
	}
	return p.Complete(fmt.Sprintf(planPrompt, facts))
}

func planFacts(res *audit.Result, sprint string) (string, error) {
	pl, err := res.PlanFor(sprint)
	if err != nil {
		return "", err
	}
	var b strings.Builder

	if pl.Sprint == "" {
		b.WriteString("Planning target: none — no dated sprint is waiting to start, so this is backlog readiness only\n")
	} else {
		fmt.Fprintf(&b, "Planning: sprint %s", pl.Sprint)
		if pl.Start != "" && pl.End != "" {
			fmt.Fprintf(&b, " (%s → %s", pl.Start, pl.End)
			if d := pl.Days(); d > 0 {
				fmt.Fprintf(&b, ", %d days", d)
			}
			if pl.State != "" {
				fmt.Fprintf(&b, ", %s", pl.State)
			}
			b.WriteString(")")
		} else {
			b.WriteString(" (no dates declared for this sprint)")
		}
		b.WriteString("\n")
	}

	if pl.From != "" {
		fmt.Fprintf(&b, "Rolling over from %s: %d story(ies)", pl.From, len(pl.Rollover))
		if pl.RolloverPoints > 0 {
			fmt.Fprintf(&b, ", %d points if all of it is pulled in", pl.RolloverPoints)
		}
		b.WriteString("\n")
		for _, s := range pl.Rollover {
			fmt.Fprintf(&b, "  Rollover: %s %q — status %s%s\n", s.ID, s.Title, s.Status, waitingNote(s))
		}
	}

	if len(pl.Committed) > 0 {
		fmt.Fprintf(&b, "Already committed to %s: %d story(ies), %d points", pl.Sprint, len(pl.Committed), pl.Points)
		if pl.Unestimated > 0 {
			fmt.Fprintf(&b, " (%d unestimated — excluded from the sum, not counted as zero)", pl.Unestimated)
		}
		b.WriteString("\n")
		for _, s := range pl.Committed {
			fmt.Fprintf(&b, "  Committed: %s %q — status %s%s%s\n", s.ID, s.Title, s.Status, pointsNote(s), waitingNote(s))
		}
	}

	for _, s := range pl.Ready {
		fmt.Fprintf(&b, "Ready to pull in: %s %q%s%s\n", s.ID, s.Title, epicNote(s), pointsNote(s))
	}
	for _, s := range pl.Blocked {
		fmt.Fprintf(&b, "Blocked: %s %q%s — waiting on %s\n", s.ID, s.Title, pointsNote(s), strings.Join(s.Waiting, ", "))
	}

	if pl.ReferenceSprint != "" {
		fmt.Fprintf(&b, "Load reference: sprint %s landed %d points. This is one prior sprint, not a velocity — there is no history behind it.\n",
			pl.ReferenceSprint, pl.Reference)
	}
	if n := len(res.Drift.StalePromises); n > 0 {
		fmt.Fprintf(&b, "Drift worth naming before committing: %d branch(es) stopped without landing\n", n)
	}
	return b.String(), nil
}

func pointsNote(s audit.PlanStory) string {
	if s.Points == 0 {
		return " (unestimated)"
	}
	return fmt.Sprintf(" (%d pts)", s.Points)
}

func epicNote(s audit.PlanStory) string {
	if s.Epic == "" {
		return ""
	}
	return " [" + s.Epic + "]"
}

func waitingNote(s audit.PlanStory) string {
	if len(s.Waiting) == 0 {
		return ""
	}
	return ", waiting on " + strings.Join(s.Waiting, ", ")
}
