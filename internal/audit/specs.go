package audit

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

const (
	Planned   Status = "planned"
	Regressed Status = "regressed"
)

// SpecStatus is a spec plus its derived (never typed) status. Epic and
// priority are carried through so agents and boards can order the backlog
// without re-reading spec files.
type SpecStatus struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owner    string   `json:"owner,omitempty"`
	Epic     string   `json:"epic,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Status   Status   `json:"status"`
	Evidence string   `json:"evidence"`
	Branches []string `json:"branches,omitempty"`
	Landed   string   `json:"landed,omitempty"` // newest trailer commit reachable from the integration branch
	File     string   `json:"file"`

	// Acceptance progress, counted from "- [ ]"/"- [x]" checkboxes in the
	// spec body — intent-side detail for boards, no file read needed there.
	AcceptanceDone  int `json:"acceptance_done,omitempty"`
	AcceptanceTotal int `json:"acceptance_total,omitempty"`
}

var checkboxPattern = regexp.MustCompile(`(?m)^\s*[-*] \[([ xX])\]`)

func acceptanceProgress(body string) (done, total int) {
	for _, m := range checkboxPattern.FindAllStringSubmatch(body, -1) {
		total++
		if m[1] != " " {
			done++
		}
	}
	return done, total
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
		ss := SpecStatus{ID: s.ID, Title: s.Title, Owner: s.Owner,
			Epic: s.Epic, Priority: s.Priority, File: s.File}
		ss.AcceptanceDone, ss.AcceptanceTotal = acceptanceProgress(s.Body)

		var linked []*Unit
		for j := range res.Units {
			u := &res.Units[j]
			if unitMatchesSpec(repo, base, s, u) {
				u.SpecID = s.ID
				linked = append(linked, u)
				ss.Branches = append(ss.Branches, u.Name)
				if creep, hit := detectScopeCreep(repo, base, s, u); hit {
					res.Drift.ScopeCreep = append(res.Drift.ScopeCreep, creep)
				}
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

	// Backlog order: priority first (unset last), then id — renderers group
	// by status, so this yields priority order inside every column.
	sort.SliceStable(res.Specs, func(i, j int) bool {
		pi, pj := rank(res.Specs[i].Priority), rank(res.Specs[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return res.Specs[i].ID < res.Specs[j].ID
	})
}

// rank treats priority 0 (unset) as lowest, not highest.
func rank(p int) int {
	if p == 0 {
		return int(^uint(0) >> 1)
	}
	return p
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

// ScopeCreep reports a spec-linked branch whose diff mostly lives outside
// the spec's declared paths — "while I was in there" work, caught before
// review.
type ScopeCreep struct {
	SpecID  string `json:"spec"`
	Branch  string `json:"branch"`
	Outside int    `json:"outside_files"`
	Total   int    `json:"total_files"`
	TopDirs string `json:"top_dirs"`
}

// detectScopeCreep flags a linked branch when more than half of its changed
// files fall outside the spec's paths. Specs without paths are never
// flagged — scope declaration is opt-in.
func detectScopeCreep(repo, base string, s *spec.Spec, u *Unit) (ScopeCreep, bool) {
	if len(s.Paths) == 0 || u.Status == Done {
		return ScopeCreep{}, false
	}
	out, ok := gitrepo.Try(repo, "diff", "--name-only", base+"..."+u.Tip)
	if !ok || out == "" {
		return ScopeCreep{}, false
	}
	var files, outside []string
	for _, f := range strings.Split(out, "\n") {
		if f == "" || strings.HasPrefix(f, ".truthboard/") {
			continue // spec edits are intent, never creep
		}
		files = append(files, f)
		if !inScope(s.Paths, f) {
			outside = append(outside, f)
		}
	}
	if len(files) == 0 || len(outside)*2 <= len(files) { // creep means >50% outside
		return ScopeCreep{}, false
	}
	return ScopeCreep{
		SpecID:  s.ID,
		Branch:  u.Name,
		Outside: len(outside),
		Total:   len(files),
		TopDirs: topDirs(outside, 3),
	}, true
}

func inScope(patterns []string, file string) bool {
	for _, p := range patterns {
		if matchScope(p, file) {
			return true
		}
	}
	return false
}

// matchScope supports the glob dialect specs actually use: `**` crosses
// directories, `*`/`?` stay within one, and a plain path with no
// metacharacters means "this file or anything under this directory".
func matchScope(pattern, file string) bool {
	if !strings.ContainsAny(pattern, "*?") {
		return file == pattern || strings.HasPrefix(file, pattern+"/")
	}
	var re strings.Builder
	re.WriteString(`\A`)
	for i := 0; i < len(pattern); i++ {
		switch {
		case strings.HasPrefix(pattern[i:], "**"):
			re.WriteString(`.*`)
			i++
		case pattern[i] == '*':
			re.WriteString(`[^/]*`)
		case pattern[i] == '?':
			re.WriteString(`[^/]`)
		default:
			re.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	re.WriteString(`\z`)
	matched, err := regexp.MatchString(re.String(), file)
	return err == nil && matched
}

// topDirs summarizes offending files by directory, biggest offenders first —
// the finding names directories, never every file.
func topDirs(files []string, n int) string {
	counts := map[string]int{}
	for _, f := range files {
		dir := path.Dir(f)
		counts[dir]++
	}
	dirs := make([]string, 0, len(counts))
	for d := range counts {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if counts[dirs[i]] != counts[dirs[j]] {
			return counts[dirs[i]] > counts[dirs[j]]
		}
		return dirs[i] < dirs[j]
	})
	if len(dirs) > n {
		dirs = dirs[:n]
	}
	parts := make([]string, len(dirs))
	for i, d := range dirs {
		parts[i] = fmt.Sprintf("%s (%d)", d, counts[d])
	}
	return strings.Join(parts, ", ")
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
