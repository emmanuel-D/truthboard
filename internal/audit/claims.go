package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/forge"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// A Claim is a tracker statement checked against repository proof.
type Claim struct {
	Kind    string `json:"kind"`    // ticket-done-but-open, ticket-stale, unticketed-work, pr-abandoned
	Subject string `json:"subject"` // #123 or branch name
	Detail  string `json:"detail"`
}

const InReview Status = "in-review"

var issueRefPattern = regexp.MustCompile(`#(\d+)`)
var fixesPattern = regexp.MustCompile(`(?i)\b(?:fix(?:es|ed)?|close[sd]?|resolve[sd]?)\s+#(\d+)`)

// refEvidence is what the repo proves about one issue number.
type refEvidence struct {
	referenced  bool // any #N mention in scanned history
	fixesMerged bool // a "fixes #N"-style commit reached the integration branch
}

// EnrichWithForge upgrades unit statuses using PR state and appends
// claim-vs-proof findings. It never downgrades a git-derived status: git
// evidence outranks tracker claims by design.
func EnrichWithForge(res *Result, data *forge.Data, opts Options) {
	if data == nil {
		return
	}
	opts = opts.normalized()
	prByHead := map[string]forge.PR{}
	for _, pr := range data.PRs {
		if _, seen := prByHead[pr.HeadRefName]; !seen { // gh lists newest first
			prByHead[pr.HeadRefName] = pr
		}
	}

	unitHasTicket := map[string]bool{}
	for i := range res.Units {
		u := &res.Units[i]
		pr, ok := prByHead[u.Name]
		if !ok {
			continue
		}
		unitHasTicket[u.Name] = true
		switch {
		case pr.State == "OPEN" && !pr.IsDraft && u.Status != Done:
			u.Status = InReview
			u.Evidence = fmt.Sprintf("PR #%d open — %s", pr.Number, u.Evidence)
		case pr.State == "OPEN" && pr.IsDraft:
			u.Evidence = fmt.Sprintf("draft PR #%d — %s", pr.Number, u.Evidence)
		case pr.State == "CLOSED" && u.Status != Done:
			res.Claims = append(res.Claims, Claim{
				Kind:    "pr-abandoned",
				Subject: u.Name,
				Detail:  fmt.Sprintf("PR #%d was closed without merging but the branch still exists", pr.Number),
			})
		}
	}

	known := map[int]bool{}
	for _, issue := range data.Issues {
		known[issue.Number] = true
	}
	evidence := collectIssueEvidence(res.Repo, res.Integration, res.Units, unitHasTicket, known, opts)

	for _, issue := range data.Issues {
		if issue.State != "OPEN" {
			continue
		}
		ev := evidence.byIssue[issue.Number]
		subject := fmt.Sprintf("#%d", issue.Number)
		switch {
		case ev.fixesMerged:
			res.Claims = append(res.Claims, Claim{
				Kind:    "ticket-done-but-open",
				Subject: subject,
				Detail: fmt.Sprintf("%q: a fixing commit already landed on %s but the ticket is still open",
					issue.Title, res.Integration),
			})
		// Only an assigned issue claims active work; unassigned open issues
		// are backlog and auditing them would drown the report in noise.
		case issue.Assigned() && !ev.referenced && int(opts.Now.Sub(issue.UpdatedAt).Hours()/24) > opts.StaleDays:
			res.Claims = append(res.Claims, Claim{
				Kind:    "ticket-stale",
				Subject: subject,
				Detail: fmt.Sprintf("%q: assigned, untouched for %dd, and never referenced by any scanned commit",
					issue.Title, int(opts.Now.Sub(issue.UpdatedAt).Hours()/24)),
			})
		}
	}

	for _, u := range res.Units {
		if u.Status == Done || u.SpecID != "" || unitHasTicket[u.Name] || evidence.unitRefs[u.Name] {
			continue
		}
		res.Claims = append(res.Claims, Claim{
			Kind:    "unticketed-work",
			Subject: u.Name,
			Detail:  "no PR and no issue reference in any of its commits — work nobody promised",
		})
	}

	res.Forge = data.Repo
}

type issueEvidence struct {
	byIssue  map[int]refEvidence
	unitRefs map[string]bool // unit branch mentions at least one issue
}

// collectIssueEvidence scans commit subjects+bodies for #N references: the
// integration branch within the digest window (merged proof) and each work
// branch's unmerged commits (intent proof). Only references to issues that
// actually exist in the tracker count — commit messages are full of
// incidental #N strings (milestones, PR numbers, "item #2").
func collectIssueEvidence(repo, base string, units []Unit, skip map[string]bool, known map[int]bool, opts Options) issueEvidence {
	ev := issueEvidence{byIssue: map[int]refEvidence{}, unitRefs: map[string]bool{}}

	if out, ok := gitrepo.Try(repo, "log", base,
		fmt.Sprintf("--since=%d.days", opts.DigestDays), "--format=%s %b%x00"); ok {
		for _, msg := range strings.Split(out, "\x00") {
			for _, n := range extract(issueRefPattern, msg) {
				e := ev.byIssue[n]
				e.referenced = true
				ev.byIssue[n] = e
			}
			for _, n := range extract(fixesPattern, msg) {
				e := ev.byIssue[n]
				e.referenced, e.fixesMerged = true, true
				ev.byIssue[n] = e
			}
		}
	}

	for _, u := range units {
		if u.Status == Done || skip[u.Name] {
			continue
		}
		out, ok := gitrepo.Try(repo, "log", "-n", "200", base+".."+u.Tip, "--format=%s %b%x00")
		if !ok {
			continue
		}
		for _, n := range extract(issueRefPattern, out) {
			if !known[n] {
				continue
			}
			ev.unitRefs[u.Name] = true
			e := ev.byIssue[n]
			e.referenced = true
			ev.byIssue[n] = e
		}
	}
	return ev
}

func extract(re *regexp.Regexp, s string) []int {
	var nums []int
	for _, m := range re.FindAllStringSubmatch(s, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}
