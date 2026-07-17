// Package audit derives work-unit statuses, a drift report, and a digest
// from git history alone. Ported from prototype/scan.py after that logic
// was validated at 100% done-vs-not-done accuracy on four real repos
// (see CONCEPT-V1.md §11).
package audit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

type Status string

const (
	InProgress Status = "in-progress"
	Stalled    Status = "stalled"
	Done       Status = "done"
)

// integrationNames are branch names that hold integrated work rather than
// representing a unit of work themselves.
var integrationNames = map[string]bool{
	"main": true, "master": true, "develop": true, "release": true, "trunk": true,
}

// mrMergePattern matches subjects of commits produced by a forge's merge
// button; such commits on the integration branch are not shadow work.
var mrMergePattern = regexp.MustCompile(`(?i)see merge request|merge branch|merge pull request`)

type Unit struct {
	Name       string    `json:"name"`
	Tip        string    `json:"tip"`
	LastCommit time.Time `json:"last_commit"`
	Status     Status    `json:"status"`
	Evidence   string    `json:"evidence"`
	Ahead      int       `json:"ahead"`
	Behind     int       `json:"behind"`
	Flags      []string  `json:"flags,omitempty"`
	SpecID     string    `json:"spec,omitempty"` // set when linked to a .truthboard spec
}

type Commit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Author  string `json:"author,omitempty"`
	Subject string `json:"subject"`
	Spec    string `json:"spec,omitempty"` // spec id this commit is attributed to
	body    string // full message, used for attribution only
}

// ShippedSpec is a promise kept inside the digest window — the digest leads
// with these, told by spec title rather than commit subject.
type ShippedSpec struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Epic  string `json:"epic,omitempty"`
	Date  string `json:"date"`
}

type Drift struct {
	StalePromises    []Unit       `json:"stale_promises"`
	LandedNotDeleted []Unit       `json:"landed_not_deleted"`
	ShadowWork       []Commit     `json:"shadow_work"`
	ScopeCreep       []ScopeCreep `json:"scope_creep,omitempty"`
}

type Result struct {
	Repo         string         `json:"repo"`
	Integration  string         `json:"integration_branch"`
	ElectedVia   string         `json:"elected_via"`
	ElectionNote string         `json:"election_note,omitempty"`
	Units        []Unit         `json:"units"`
	Drift        Drift          `json:"drift"`
	Digest       []Commit       `json:"digest"`
	Shipped      []ShippedSpec  `json:"shipped,omitempty"` // specs landed within the digest window
	Sprints      []SprintRollup `json:"sprints,omitempty"` // per-sprint arithmetic over derived statuses
	Specs        []SpecStatus   `json:"specs,omitempty"`
	Claims       []Claim        `json:"claims,omitempty"`
	Forge        string         `json:"forge,omitempty"` // owner/name when forge data enriched the audit
	StaleDays    int            `json:"stale_days"`
	DigestDays   int            `json:"digest_days"`
	GeneratedAt  time.Time      `json:"generated_at"`
}

type Options struct {
	StaleDays  int
	DigestDays int
	Now        time.Time // zero value means time.Now()
}

type branchTip struct {
	sha  string
	when time.Time
}

func (o Options) normalized() Options {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.StaleDays <= 0 {
		o.StaleDays = 7
	}
	if o.DigestDays <= 0 {
		o.DigestDays = 14
	}
	return o
}

// Audit runs the full read-only analysis of repo.
func Audit(repo string, opts Options) (*Result, error) {
	opts = opts.normalized()

	branches, err := collectBranches(repo)
	if err != nil {
		return nil, err
	}
	elected, via, note, err := electIntegration(repo, branches)
	if err != nil {
		return nil, err
	}
	base := integrationRef(repo, elected)

	// A branch whose tip IS the integration tip has no work of its own —
	// it's a freshly cut branch, not a merged one, and must not read as
	// done (nor as anything else: intent without commits is the spec's
	// planned state, not the branch board's business).
	baseSHA, _ := gitrepo.Try(repo, "rev-parse", base)

	names := make([]string, 0, len(branches))
	for name := range branches {
		if integrationNames[name] || name == elected || branches[name].sha == baseSHA {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	units := make([]Unit, 0, len(names))
	for _, name := range names {
		units = append(units, classify(repo, base, name, branches[name], opts))
	}

	specs, err := spec.Load(repo)
	if err != nil {
		return nil, err
	}
	sprintIntents, err := spec.LoadSprints(repo)
	if err != nil {
		return nil, err
	}

	shadow, err := shadowWork(repo, base, opts.DigestDays)
	if err != nil {
		return nil, err
	}
	dig, err := digest(repo, base, opts.DigestDays)
	if err != nil {
		return nil, err
	}

	res := &Result{
		Repo:         repo,
		Integration:  base,
		ElectedVia:   via,
		ElectionNote: note,
		Units:        units,
		Digest:       dig,
		StaleDays:    opts.StaleDays,
		DigestDays:   opts.DigestDays,
		GeneratedAt:  opts.Now,
	}
	linkSpecs(repo, base, res, specs, opts)
	attributeDigest(res)
	rollupSprints(res, sprintIntents, opts.Now)
	for _, u := range res.Units {
		switch u.Status {
		case Stalled:
			res.Drift.StalePromises = append(res.Drift.StalePromises, u)
		case Done:
			res.Drift.LandedNotDeleted = append(res.Drift.LandedNotDeleted, u)
		}
	}
	res.Drift.ShadowWork = shadow
	return res, nil
}

// collectBranches gathers local and origin branches, deduplicated by short
// name, keeping whichever tip is newest.
func collectBranches(repo string) (map[string]branchTip, error) {
	out, err := gitrepo.Run(repo, "for-each-ref", "refs/heads", "refs/remotes/origin",
		"--format=%(refname)|%(objectname)|%(committerdate:iso8601-strict)")
	if err != nil {
		return nil, err
	}
	branches := map[string]branchTip{}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 || strings.HasSuffix(parts[0], "/HEAD") {
			continue
		}
		name := strings.TrimPrefix(strings.TrimPrefix(parts[0], "refs/heads/"), "refs/remotes/origin/")
		when, err := time.Parse(time.RFC3339, parts[2])
		if err != nil {
			return nil, fmt.Errorf("parsing commit date for %s: %w", parts[0], err)
		}
		if cur, ok := branches[name]; !ok || when.After(cur.when) {
			branches[name] = branchTip{sha: parts[1], when: when}
		}
	}
	if len(branches) == 0 {
		return nil, fmt.Errorf("no branches found in %s", repo)
	}
	return branches, nil
}

// electIntegration picks the integration branch. origin/HEAD is the first
// hint, but a stale remote default must not poison every inference, so among
// integration-named candidates the most recently active tip wins.
func electIntegration(repo string, branches map[string]branchTip) (name, via, note string, err error) {
	hint := ""
	if ref, ok := gitrepo.Try(repo, "symbolic-ref", "refs/remotes/origin/HEAD"); ok {
		hint = ref[strings.LastIndex(ref, "/")+1:]
	}
	elected, newest := "", time.Time{}
	for n, tip := range branches {
		if !integrationNames[n] && n != hint {
			continue
		}
		if tip.when.After(newest) || (tip.when.Equal(newest) && n < elected) {
			elected, newest = n, tip.when
		}
	}
	if elected == "" {
		return "", "", "", fmt.Errorf("cannot determine integration branch in %s", repo)
	}
	if hint != "" && elected != hint {
		hintDate := "unknown"
		if tip, ok := branches[hint]; ok {
			hintDate = tip.when.Format("2006-01-02")
		}
		note = fmt.Sprintf("origin/HEAD points to %q (last active %s) but %q is newer — remote default branch looks misconfigured",
			hint, hintDate, elected)
		return elected, "activity election", note, nil
	}
	via = "activity election"
	if elected == hint {
		via = "origin/HEAD"
	}
	return elected, via, "", nil
}

// integrationRef prefers the remote-tracking ref so a stale local checkout
// of the integration branch doesn't skew inference.
func integrationRef(repo, name string) string {
	if _, ok := gitrepo.Try(repo, "rev-parse", "--verify", "origin/"+name); ok {
		return "origin/" + name
	}
	return name
}

func classify(repo, base, name string, tip branchTip, opts Options) Unit {
	u := Unit{Name: name, Tip: tip.sha, LastCommit: tip.when}

	if _, ok := gitrepo.Try(repo, "merge-base", "--is-ancestor", tip.sha, base); ok {
		u.Status = Done
		u.Evidence = fmt.Sprintf("tip is ancestor of %s (merged)", base)
		return u
	}

	// Squash/rebase merges leave no ancestry; git cherry marks
	// patch-equivalent commits with '-'.
	if out, ok := gitrepo.Try(repo, "cherry", base, tip.sha); ok && out != "" {
		lines := strings.Split(out, "\n")
		equivalent := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "-") {
				equivalent++
			}
		}
		if equivalent == len(lines) {
			u.Status = Done
			u.Evidence = fmt.Sprintf("all %d commits patch-equivalent in %s (squash/rebase merge)", len(lines), base)
			return u
		}
		if equivalent > 0 {
			u.Flags = append(u.Flags, fmt.Sprintf("%d/%d commits already in %s (partial merge)", equivalent, len(lines), base))
		}
	}

	if out, ok := gitrepo.Try(repo, "rev-list", "--left-right", "--count", base+"..."+tip.sha); ok {
		fmt.Sscanf(out, "%d %d", &u.Behind, &u.Ahead)
	}

	age := int(opts.Now.Sub(tip.when).Hours() / 24)
	if age > opts.StaleDays {
		u.Status = Stalled
		u.Evidence = fmt.Sprintf("no commits for %d days (%d unmerged)", age, u.Ahead)
	} else {
		u.Status = InProgress
		u.Evidence = fmt.Sprintf("active %dd ago, %d commits ahead, %d behind", age, u.Ahead, u.Behind)
	}
	return u
}

// shadowWork returns non-merge commits landing directly on the integration
// branch that don't look like a forge merge — work that bypassed any
// branch/MR flow.
func shadowWork(repo, base string, days int) ([]Commit, error) {
	out, err := gitrepo.Run(repo, "log", base, "--first-parent", "--no-merges",
		fmt.Sprintf("--since=%d.days", days), "--format=%h|%cs|%an|%s")
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 || mrMergePattern.MatchString(parts[3]) {
			continue
		}
		commits = append(commits, Commit{Hash: parts[0], Date: parts[1], Author: parts[2], Subject: parts[3]})
	}
	return commits, nil
}

func digest(repo, base string, days int) ([]Commit, error) {
	out, err := gitrepo.Run(repo, "log", base, "--first-parent",
		fmt.Sprintf("--since=%d.days", days), "--format=%h%x00%cs%x00%s%x00%B%x1e")
	if err != nil {
		return nil, err
	}
	var commits []Commit
	for _, entry := range strings.Split(out, "\x1e") {
		parts := strings.SplitN(strings.TrimSpace(entry), "\x00", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{Hash: parts[0], Date: parts[1], Subject: parts[2], body: parts[3]})
	}
	return commits, nil
}

// attributeDigest links digest commits to the specs they mention (trailer
// or id anywhere in the message) and lifts done specs that landed inside
// the window into the Shipped list — the digest's headline. Commits no
// spec claims stay unattributed and render as "also landed".
func attributeDigest(res *Result) {
	if len(res.Specs) == 0 {
		return
	}
	shipped := map[string]bool{}
	for i := range res.Digest {
		c := &res.Digest[i]
		for j := range res.Specs {
			s := &res.Specs[j]
			if !strings.Contains(c.body, s.ID) {
				continue
			}
			c.Spec = s.ID
			if s.Status == Done && !shipped[s.ID] {
				shipped[s.ID] = true
				res.Shipped = append(res.Shipped, ShippedSpec{
					ID: s.ID, Title: s.Title, Epic: s.Epic, Date: c.Date,
				})
			}
			break
		}
		c.body = ""
	}
}
