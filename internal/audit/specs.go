package audit

import (
	"fmt"
	"path"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

const Planned Status = "planned"

// SpecStatus is a spec plus its derived (never typed) status.
type SpecStatus struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owner    string   `json:"owner,omitempty"`
	Status   Status   `json:"status"`
	Evidence string   `json:"evidence"`
	Branches []string `json:"branches,omitempty"`
	File     string   `json:"file"`
}

// linkSpecs matches every spec against repo reality and appends derived
// spec statuses to the result. Linking signals, strongest first: a
// "Spec: <id>" commit trailer, the id appearing in a branch name, the
// spec's branch glob.
func linkSpecs(repo, base string, res *Result, specs []spec.Spec, opts Options) {
	for i := range specs {
		s := &specs[i]
		ss := SpecStatus{ID: s.ID, Title: s.Title, Owner: s.Owner, File: s.File}

		var linked []*Unit
		for j := range res.Units {
			u := &res.Units[j]
			if unitMatchesSpec(repo, base, s, u) {
				u.SpecID = s.ID
				linked = append(linked, u)
				ss.Branches = append(ss.Branches, u.Name)
			}
		}
		merged := trailerMergedInto(repo, base, s)
		ss.Status, ss.Evidence = deriveSpecStatus(linked, merged, base)
		res.Specs = append(res.Specs, ss)
	}
}

func unitMatchesSpec(repo, base string, s *spec.Spec, u *Unit) bool {
	if strings.Contains(u.Name, s.ID) {
		return true
	}
	if s.Branch != "" {
		if ok, _ := path.Match(s.Branch, u.Name); ok {
			return true
		}
	}
	// Trailer in any unmerged commit of the branch.
	out, ok := gitrepo.Try(repo, "log", "-n", "200", "--grep", s.Trailer(), base+".."+u.Tip, "--format=%h")
	return ok && out != ""
}

// trailerMergedInto reports whether any commit carrying the spec's trailer
// is already reachable from the integration branch.
func trailerMergedInto(repo, base string, s *spec.Spec) bool {
	out, ok := gitrepo.Try(repo, "log", "-n", "1", "--grep", s.Trailer(), base, "--format=%h")
	return ok && out != ""
}

// deriveSpecStatus rolls linked-branch statuses up to the spec.
// Active-work states outrank done: landing part of the work while a linked
// branch still moves means the spec is not finished.
func deriveSpecStatus(linked []*Unit, trailerMerged bool, base string) (Status, string) {
	var inReview, inProgress, stalled, done []*Unit
	for _, u := range linked {
		switch u.Status {
		case InReview:
			inReview = append(inReview, u)
		case InProgress:
			inProgress = append(inProgress, u)
		case Stalled:
			stalled = append(stalled, u)
		case Done:
			done = append(done, u)
		}
	}
	switch {
	case len(inReview) > 0:
		return InReview, fmt.Sprintf("%s — %s", inReview[0].Name, inReview[0].Evidence)
	case len(inProgress) > 0:
		return InProgress, fmt.Sprintf("%s — %s", inProgress[0].Name, inProgress[0].Evidence)
	case trailerMerged || len(done) > 0:
		evidence := fmt.Sprintf("work landed on %s", base)
		if len(stalled) > 0 {
			evidence += fmt.Sprintf(" — but %s still has unmerged commits", stalled[0].Name)
		}
		return Done, evidence
	case len(stalled) > 0:
		return Stalled, fmt.Sprintf("%s — %s", stalled[0].Name, stalled[0].Evidence)
	default:
		return Planned, "no matching branch or commit yet"
	}
}
