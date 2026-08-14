package audit

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/spec"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// acceptanceInstructions is the half of the definition of done that git
// cannot derive. The body above already shows the checklist as markdown;
// this repeats it numbered, because a criterion you can name in one word
// ("check 3") is a criterion that gets recorded, and adds the closing step
// itself — the one step every landed-but-0/3 story on this board skipped.
func acceptanceInstructions(s *spec.Spec) string {
	cs := s.Acceptance()
	if len(cs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nAcceptance criteria, numbered as the tick verbs address them:\n\n%s", spec.Checklist(cs))
	fmt.Fprintf(&b, "\nAs each one becomes true, record it — this is part of the task, not paperwork\n")
	fmt.Fprintf(&b, "after it: check_acceptance (MCP) or `truthboard check %s <n|substring|all>`, then\n", s.ID)
	fmt.Fprintf(&b, "commit the tick with the same trailer. Do not tick what you did not verify:\n")
	fmt.Fprintf(&b, "an unticked criterion is a question, a wrongly ticked one is a false answer.\n")
	fmt.Fprintf(&b, "The status is git's either way — ticking never sets it, and never blocks it.\n")
	return b.String()
}

// Brief renders the context packet an agent (or human) needs to start
// working a spec: the intent, the linking instructions, and the current
// derived status.
func Brief(repo, id string) (string, error) {
	s, err := spec.Find(repo, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Analyze and resolve spec %s in this repository.\n\n---\n", s.ID)
	fmt.Fprintf(&b, "Title: %s\n", s.Title)
	if s.Owner != "" {
		fmt.Fprintf(&b, "Owner: %s\n", s.Owner)
	}
	if len(s.Paths) > 0 {
		fmt.Fprintf(&b, "Scope: %s (work outside these paths will be reported as scope creep)\n",
			strings.Join(s.Paths, ", "))
	}
	fmt.Fprintf(&b, "\n%s\n---\n\n", s.Body)
	fmt.Fprintf(&b, "Work on a branch matching %q (or any branch containing %q).\n", s.Branch, s.ID)
	fmt.Fprintf(&b, "End every commit message with the trailer:\n\n    %s\n\n", s.Trailer())
	fmt.Fprintf(&b, "Satisfy the acceptance criteria while maintaining code health.\n")
	b.WriteString(acceptanceInstructions(s))

	// In a workspace, the split-or-declare choice belongs in the brief:
	// the agent picking up a fat story is the one who should decompose it,
	// never the PO who wrote it on a phone.
	if ws, _ := workspace.Load(repo); ws != nil && len(ws.Repos) > 0 {
		names := make([]string, len(ws.Repos))
		for i, r := range ws.Repos {
			names[i] = r.Name
		}
		fmt.Fprintf(&b, "\nWorkspace: this hub gathers proof from %s (and the hub itself); branches and trailers link the same way in every one.\n",
			strings.Join(names, ", "))
		if len(s.Repos) > 0 {
			fmt.Fprintf(&b, "This story declares repos: %s — it derives done only when the trailer has landed on the integration branch of every one; the board shows per-repo evidence until then.\n",
				strings.Join(s.Repos, ", "))
		} else {
			fmt.Fprintf(&b, "If the acceptance spans more than one of these repos, split before coding: narrow this story to one repo's half (update_spec) and create_spec a sibling per remaining repo (same epic, needs: for ordering) — or, if it must land everywhere as one promise, declare repos: [...] on it. Never leave a story no branch will ever match.\n")
		}
	}

	if res, err := Audit(repo, Options{}); err == nil {
		for _, ss := range res.Specs {
			if ss.ID != s.ID {
				continue
			}
			if ss.Status != Planned {
				fmt.Fprintf(&b, "Current derived status: %s (%s)\n", ss.Status, ss.Evidence)
			}
			// Whoever picks this up must know it was paused, and must not
			// be told a reason git has already disproved.
			if ss.Hold != "" {
				if ss.HoldContradicted != "" {
					fmt.Fprintf(&b, "Hold note (contradicted — %s): %s\n", ss.HoldContradicted, ss.Hold)
				} else {
					fmt.Fprintf(&b, "On hold: %s\n", ss.Hold)
				}
			}
		}
	}
	return b.String(), nil
}
