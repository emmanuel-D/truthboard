package llm

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

const reviewPrompt = `You are writing a sprint review for a software team.
Below are the derived facts from their git repository — statuses come from
commits and merges, not from anyone's claims. Write a concise, honest
narrative review in markdown: what landed and why it matters, what is still
open and appears to roll over, and any drift worth naming. No invented
work, no praise padding, no facts beyond those given. 150-300 words.

%s`

// Review narrates the sprint (or, with an empty sprint slug, the whole
// digest window) from the derived Result. The LLM only ever rephrases
// facts the audit already established — it is a writer, not a source.
func Review(p Provider, res *audit.Result, sprint string) (string, error) {
	facts, err := reviewFacts(res, sprint)
	if err != nil {
		return "", err
	}
	return p.Complete(fmt.Sprintf(reviewPrompt, facts))
}

func reviewFacts(res *audit.Result, sprint string) (string, error) {
	var b strings.Builder
	if sprint != "" {
		var found *audit.SprintRollup
		for i := range res.Sprints {
			if res.Sprints[i].Name == sprint {
				found = &res.Sprints[i]
				break
			}
		}
		if found == nil {
			return "", fmt.Errorf("no stories carry sprint %q", sprint)
		}
		fmt.Fprintf(&b, "Sprint: %s — %d/%d stories done", found.Name, found.Done, found.Total)
		if found.PointsTotal > 0 {
			fmt.Fprintf(&b, ", %d/%d points", found.PointsDone, found.PointsTotal)
		}
		if found.State != "" {
			fmt.Fprintf(&b, " (%s → %s, %s)", found.Start, found.End, found.State)
		}
		b.WriteString("\n")
		for _, o := range found.Open {
			fmt.Fprintf(&b, "Still open: %s %q — status %s\n", o.ID, o.Title, o.Status)
		}
		for _, s := range res.Specs {
			if s.Sprint == sprint && s.Status == audit.Done {
				fmt.Fprintf(&b, "Landed: %s %q\n", s.ID, s.Title)
			}
		}
	} else {
		fmt.Fprintf(&b, "Window: last %d days on %s\n", res.DigestDays, res.Integration)
		for _, sh := range res.Shipped {
			kind := sh.Type
			if kind == "" {
				kind = "story"
			}
			fmt.Fprintf(&b, "Landed: %s %q (%s, %s)\n", sh.ID, sh.Title, kind, sh.Date)
		}
	}
	if n := len(res.Drift.StalePromises); n > 0 {
		fmt.Fprintf(&b, "Drift: %d branch(es) stopped without landing:", n)
		for _, u := range res.Drift.StalePromises {
			fmt.Fprintf(&b, " %s (%s);", u.Name, u.Evidence)
		}
		b.WriteString("\n")
	}
	if n := len(res.Drift.ShadowWork); n > 0 {
		fmt.Fprintf(&b, "Drift: %d commit(s) landed outside any branch/spec flow\n", n)
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("nothing to review — no landed work or open stories in scope")
	}
	return b.String(), nil
}
