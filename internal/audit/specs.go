package audit

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

const (
	Planned   Status = "planned"
	Regressed Status = "regressed"
)

// SpecStatus is a spec plus its derived (never typed) status.
type SpecStatus struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owner    string   `json:"owner,omitempty"`
	Status   Status   `json:"status"`
	Evidence string   `json:"evidence"`
	Branches []string `json:"branches,omitempty"`
	Landed   string   `json:"landed,omitempty"` // newest trailer commit reachable from the integration branch
	File     string   `json:"file"`
}

// linkSpecs matches every spec against repo reality and appends derived
// spec statuses to the result. Linking signals, strongest first: a
// "Spec: <id>" commit trailer, the id appearing in a branch name, the
// spec's branch glob.
func linkSpecs(repo, base string, res *Result, specs []spec.Spec, opts Options) {
	var reverts []revertInfo
	revertsLoaded := false

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
		ss.Landed = landingCommit(repo, base, s)
		ss.Status, ss.Evidence = deriveSpecStatus(linked, ss.Landed != "", base)

		// A done spec must loudly regress when its landed work is reverted.
		if ss.Status == Done {
			if !revertsLoaded {
				reverts, revertsLoaded = collectReverts(repo, base, opts.DigestDays), true
			}
			if ev, hit := regressionEvidence(repo, s, reverts); hit {
				ss.Status, ss.Evidence = Regressed, ev
			}
		}
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

// landingCommit returns the newest commit carrying the spec's trailer that
// is reachable from the integration branch, or "" when none landed yet.
// This SHA is also what a CI check (forge enrichment) is run against.
func landingCommit(repo, base string, s *spec.Spec) string {
	out, ok := gitrepo.Try(repo, "log", "-n", "1", "--grep", s.Trailer(), base, "--format=%H")
	if !ok {
		return ""
	}
	return out
}

type revertInfo struct {
	revertSHA   string
	revertedSHA string
}

var revertedShaPattern = regexp.MustCompile(`This reverts commit ([0-9a-f]{7,40})`)

// collectReverts finds revert commits that recently landed on the
// integration branch and the SHAs they undo. The window applies to the
// revert itself, so old work reverted today is still caught.
func collectReverts(repo, base string, days int) []revertInfo {
	out, ok := gitrepo.Try(repo, "log", base, fmt.Sprintf("--since=%d.days", days),
		"--grep", "This reverts commit", "--format=%H%x00%B%x1e")
	if !ok || out == "" {
		return nil
	}
	var reverts []revertInfo
	for _, entry := range strings.Split(out, "\x1e") {
		parts := strings.SplitN(strings.TrimSpace(entry), "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		for _, m := range revertedShaPattern.FindAllStringSubmatch(parts[1], -1) {
			reverts = append(reverts, revertInfo{revertSHA: parts[0], revertedSHA: m[1]})
		}
	}
	return reverts
}

// regressionEvidence reports whether any reverted commit belonged to the
// spec — its message carries the spec's trailer, or mentions the spec id
// (which also covers reverts of "Merge branch 'feature/<id>-…'" commits).
func regressionEvidence(repo string, s *spec.Spec, reverts []revertInfo) (string, bool) {
	for _, r := range reverts {
		msg, ok := gitrepo.Try(repo, "show", "-s", "--format=%B", r.revertedSHA)
		if !ok {
			continue
		}
		if strings.Contains(msg, s.Trailer()) || strings.Contains(msg, s.ID) {
			return fmt.Sprintf("landed work was reverted by %.7s (reverts %.7s)", r.revertSHA, r.revertedSHA), true
		}
	}
	return "", false
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
