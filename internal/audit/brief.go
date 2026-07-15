package audit

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

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

	if res, err := Audit(repo, Options{}); err == nil {
		for _, ss := range res.Specs {
			if ss.ID == s.ID && ss.Status != Planned {
				fmt.Fprintf(&b, "Current derived status: %s (%s)\n", ss.Status, ss.Evidence)
			}
		}
	}
	return b.String(), nil
}
