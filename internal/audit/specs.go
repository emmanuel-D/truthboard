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
	ID         string        `json:"id"`
	Title      string        `json:"title"`
	Owner      string        `json:"owner,omitempty"`
	Epic       string        `json:"epic,omitempty"`
	Sprint     string        `json:"sprint,omitempty"`
	Priority   int           `json:"priority,omitempty"`
	Points     int           `json:"points,omitempty"`
	Type       string        `json:"type,omitempty"`
	Needs      []string      `json:"needs,omitempty"`   // declared prerequisites (intent)
	Waiting    []string      `json:"waiting,omitempty"` // the subset of needs not yet done — derived
	Repos      []string      `json:"repos,omitempty"`   // declared required repos (intent, from frontmatter)
	Status     Status        `json:"status"`
	Evidence   string        `json:"evidence"`
	Branches   []string      `json:"branches,omitempty"`
	Landed     string        `json:"landed,omitempty"`      // newest trailer commit reachable from the integration branch
	LandedRepo string        `json:"landed_repo,omitempty"` // workspace repo the landing commit is in; empty means the hub
	PerRepo    []RepoLanding `json:"per_repo,omitempty"`    // derived state per declared repo, in declaration order
	File       string        `json:"file"`

	// Acceptance progress, counted from "- [ ]"/"- [x]" checkboxes in the
	// spec body — intent-side detail for boards, no file read needed there.
	AcceptanceDone  int `json:"acceptance_done,omitempty"`
	AcceptanceTotal int `json:"acceptance_total,omitempty"`
}

// RepoLanding is one declared repo's derived state for a spec with repos:
// intent — the board's per-repo evidence chip, structured.
type RepoLanding struct {
	Repo   string `json:"repo"`             // "hub" or a spoke name, as declared
	State  string `json:"state"`            // landed | in-review | in-progress | stalled | missing | unreadable | not-in-workspace
	SHA    string `json:"sha,omitempty"`    // landing commit when state is landed
	Branch string `json:"branch,omitempty"` // strongest live branch otherwise
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

// linkSpecs matches every spec against repo reality — the hub and every
// resolvable spoke — and appends derived spec statuses to the result.
// Linking signals, strongest first: a "Spec: <id>" commit trailer, the id
// appearing in a branch name, the spec's branch glob. The id namespace is
// global because intent is central, so the same signals work unchanged in
// every repo.
func linkSpecs(ctxs []repoCtx, res *Result, specs []spec.Spec, opts Options) {
	byName := map[string]repoCtx{}
	for _, ctx := range ctxs {
		byName[ctx.name] = ctx
	}
	reverts := map[string][]revertInfo{}

	for i := range specs {
		s := &specs[i]
		ss := SpecStatus{ID: s.ID, Title: s.Title, Owner: s.Owner,
			Epic: s.Epic, Sprint: s.Sprint, Priority: s.Priority, Points: s.Points, Type: s.Type, Needs: s.Needs, File: s.File}
		ss.AcceptanceDone, ss.AcceptanceTotal = acceptanceProgress(s.Body)

		var linked []*Unit
		for j := range res.Units {
			u := &res.Units[j]
			ctx := byName[u.Repo]
			if unitMatchesSpec(ctx.path, ctx.base, s, u) {
				u.SpecID = s.ID
				linked = append(linked, u)
				ss.Branches = append(ss.Branches, u.Label())
				if creep, hit := detectScopeCreep(ctx, s, u); hit {
					res.Drift.ScopeCreep = append(res.Drift.ScopeCreep, creep)
				}
			}
		}
		if len(s.Repos) > 0 {
			ss.Repos = s.Repos
			ss.Status, ss.Evidence = deriveDeclaredRepos(byName, s, &ss, linked, res)
		} else {
			landedLabel := ""
			for _, ctx := range ctxs {
				if sha := landingCommit(ctx.path, ctx.base, s); sha != "" {
					ss.Landed, ss.LandedRepo = sha, ctx.name
					landedLabel = ctx.label(ctx.base)
					break
				}
			}
			ss.Status, ss.Evidence = deriveSpecStatus(byName, linked, ss.Landed != "", landedLabel)
		}

		// A done spec must loudly regress when its landed work is reverted —
		// in whichever repo it landed or was undone.
		if ss.Status == Done {
			for _, ctx := range ctxs {
				if _, ok := reverts[ctx.name]; !ok {
					reverts[ctx.name] = collectReverts(ctx.path, ctx.base, opts.DigestDays)
				}
				if ev, hit := regressionEvidence(ctx.path, s, reverts[ctx.name]); hit {
					if ctx.name != "" {
						ev = ctx.name + ": " + ev
					}
					ss.Status, ss.Evidence = Regressed, ev
					break
				}
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
	// Same glob dialect as spec paths (matchScope): ** crosses slashes,
	// * stays within one segment — one dialect everywhere.
	if s.Branch != "" && matchScope(s.Branch, u.Name) {
		return true
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
// flagged — scope declaration is opt-in, per repo: an `api:` prefix scopes
// a pattern to that spoke, an unprefixed pattern to the hub. A branch in a
// repo the spec declares no paths for is never flagged either.
func detectScopeCreep(ctx repoCtx, s *spec.Spec, u *Unit) (ScopeCreep, bool) {
	paths := pathsFor(ctx.name, s.Paths)
	if len(paths) == 0 || u.Status == Done {
		return ScopeCreep{}, false
	}
	out, ok := gitrepo.Try(ctx.path, "diff", "--name-only", ctx.base+"..."+u.Tip)
	if !ok || out == "" {
		return ScopeCreep{}, false
	}
	var files, outside []string
	for _, f := range strings.Split(out, "\n") {
		if f == "" || strings.HasPrefix(f, ".truthboard/") {
			continue // spec edits are intent, never creep
		}
		files = append(files, f)
		if !inScope(paths, f) {
			outside = append(outside, f)
		}
	}
	if len(files) == 0 || len(outside)*2 <= len(files) { // creep means >50% outside
		return ScopeCreep{}, false
	}
	return ScopeCreep{
		SpecID:  s.ID,
		Branch:  u.Label(),
		Outside: len(outside),
		Total:   len(files),
		TopDirs: topDirs(outside, 3),
	}, true
}

// pathsFor selects the spec path patterns that apply to one repo: a
// "name:" prefix (matching the workspace name grammar) binds a pattern to
// that spoke; everything else belongs to the hub.
func pathsFor(repoName string, patterns []string) []string {
	var out []string
	for _, p := range patterns {
		name, rest, prefixed := splitRepoPrefix(p)
		switch {
		case prefixed && name == repoName:
			out = append(out, rest)
		case !prefixed && repoName == "":
			out = append(out, p)
		}
	}
	return out
}

var repoPrefixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func splitRepoPrefix(p string) (name, rest string, ok bool) {
	i := strings.Index(p, ":")
	if i <= 0 || i == len(p)-1 || !repoPrefixPattern.MatchString(p[:i]) {
		return "", "", false
	}
	// Git paths are repo-relative: a "/" after the colon means this is an
	// URL ("https://…"), not a repo-scoped pattern.
	if strings.HasPrefix(p[i+1:], "/") {
		return "", "", false
	}
	return p[:i], p[i+1:], true
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

// deriveDeclaredRepos derives status for a spec that declares repos:
// intent — done requires the trailer landed on the integration branch of
// EVERY declared repo; git cannot prove the absence of work it never knew
// was intended, so this is the one place multi-repo runs on declared
// intent. Evidence is per-repo chips ("api ✓ landed · web — no branch
// yet") — a partially landed story must say exactly what is missing, never
// a mute in-progress. A declared repo the workspace doesn't know is a
// drift finding and keeps the spec from ever deriving done.
func deriveDeclaredRepos(byName map[string]repoCtx, s *spec.Spec, ss *SpecStatus, linked []*Unit, res *Result) (Status, string) {
	unreadable := map[string]bool{}
	for _, h := range res.Workspace {
		if h.Err != "" {
			unreadable[h.Name] = true
		}
	}
	// Strongest live unit per repo: in-review > in-progress > stalled.
	strength := map[Status]int{InReview: 3, InProgress: 2, Stalled: 1}
	strongest := map[string]*Unit{}
	for _, u := range linked {
		if cur := strongest[u.Repo]; strength[u.Status] > 0 && (cur == nil || strength[u.Status] > strength[cur.Status]) {
			strongest[u.Repo] = u
		}
	}

	declared := map[string]bool{}
	landedAll, landedAny := true, false
	var chips []string
	for _, name := range s.Repos {
		key := name // internal ctx key: hub is ""
		if name == spec.HubRepo {
			key = ""
		}
		declared[key] = true
		ctx, ok := byName[key]
		if !ok {
			landedAll = false
			if unreadable[name] {
				chips = append(chips, name+" ✗ unreadable")
				ss.PerRepo = append(ss.PerRepo, RepoLanding{Repo: name, State: "unreadable"})
			} else {
				res.Drift.UnknownRepos = append(res.Drift.UnknownRepos,
					fmt.Sprintf("%s declares repos: %q — not in the workspace manifest", s.ID, name))
				chips = append(chips, name+" ✗ not in workspace")
				ss.PerRepo = append(ss.PerRepo, RepoLanding{Repo: name, State: "not-in-workspace"})
			}
			continue
		}
		if sha := landingCommit(ctx.path, ctx.base, s); sha != "" {
			landedAny = true
			chips = append(chips, name+" ✓ landed")
			ss.PerRepo = append(ss.PerRepo, RepoLanding{Repo: name, State: "landed", SHA: sha})
			// Landed/LandedRepo keep single-landing consumers working (CI
			// checks run only against a hub landing).
			if name == spec.HubRepo {
				ss.Landed, ss.LandedRepo = sha, ""
			} else if ss.Landed == "" {
				ss.Landed, ss.LandedRepo = sha, name
			}
			continue
		}
		landedAll = false
		if u := strongest[key]; u != nil {
			chips = append(chips, fmt.Sprintf("%s — %s (%s)", name, u.Status, u.Name))
			ss.PerRepo = append(ss.PerRepo, RepoLanding{Repo: name, State: string(u.Status), Branch: u.Name})
		} else {
			chips = append(chips, name+" — no branch yet")
			ss.PerRepo = append(ss.PerRepo, RepoLanding{Repo: name, State: "missing"})
		}
	}
	evidence := strings.Join(chips, " · ")

	// Live work anywhere — declared or not — still outranks landings; a
	// branch moving in an undeclared repo is prefixed so the chips don't
	// hide it.
	var review, active, stalled *Unit
	for _, u := range linked {
		switch u.Status {
		case InReview:
			if review == nil {
				review = u
			}
		case InProgress:
			if active == nil {
				active = u
			}
		case Stalled:
			if stalled == nil {
				stalled = u
			}
		}
	}
	prefix := ""
	if lead := review; lead != nil || active != nil {
		if lead == nil {
			lead = active
		}
		if !declared[lead.Repo] {
			prefix = fmt.Sprintf("%s — %s · ", lead.Label(), lead.Evidence)
		}
	}

	switch {
	case review != nil:
		return InReview, prefix + evidence
	case active != nil:
		return InProgress, prefix + evidence
	case landedAll:
		if stalled != nil {
			evidence += fmt.Sprintf(" — but %s still has unmerged commits", stalled.Label())
		}
		return Done, evidence
	case stalled != nil:
		return Stalled, evidence
	case landedAny:
		return InProgress, evidence
	default:
		return Planned, evidence
	}
}

// deriveSpecStatus rolls linked-branch statuses up to the spec.
// Active-work states outrank done: landing part of the work while a linked
// branch still moves — in any repo of the workspace — means the spec is not
// finished. landedLabel names where the trailer landed ("origin/main",
// "api:main"); when only merged branches prove the landing, the label comes
// from the first such branch's repo.
func deriveSpecStatus(byName map[string]repoCtx, linked []*Unit, trailerMerged bool, landedLabel string) (Status, string) {
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
		return InReview, fmt.Sprintf("%s — %s", inReview[0].Label(), inReview[0].Evidence)
	case len(inProgress) > 0:
		return InProgress, fmt.Sprintf("%s — %s", inProgress[0].Label(), inProgress[0].Evidence)
	case trailerMerged || len(done) > 0:
		if landedLabel == "" {
			ctx := byName[done[0].Repo]
			landedLabel = ctx.label(ctx.base)
		}
		evidence := fmt.Sprintf("work landed on %s", landedLabel)
		if len(stalled) > 0 {
			evidence += fmt.Sprintf(" — but %s still has unmerged commits", stalled[0].Label())
		}
		return Done, evidence
	case len(stalled) > 0:
		return Stalled, fmt.Sprintf("%s — %s", stalled[0].Label(), stalled[0].Evidence)
	default:
		return Planned, "no matching branch or commit yet"
	}
}
