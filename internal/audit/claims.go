package audit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/forge"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
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
// claim-vs-proof findings for the hub repo alone. It never downgrades a
// git-derived status: git evidence outranks tracker claims by design.
func EnrichWithForge(res *Result, data *forge.Data, opts Options) {
	enrichRepoForge(res, repoCtx{path: res.Repo, base: res.Integration}, data, opts)
}

// EnrichWithForges runs forge enrichment across the whole workspace: the
// hub first, then every readable spoke. Each repo's own remote decides
// which forge answers for it (fetch is forge.Fetch in production —
// gh/glab auto-detect per repo). A spoke whose forge is unreachable or
// unauthenticated keeps its git-only derivation and carries a visible
// note — degraded, never silent (the sync-freshness honesty rule).
func EnrichWithForges(res *Result, fetch func(path string) (*forge.Data, bool), opts Options) {
	if fetch == nil {
		return
	}
	if data, ok := fetch(res.Repo); ok {
		EnrichWithForge(res, data, opts)
	}
	for i := range res.Workspace {
		h := &res.Workspace[i]
		if h.Err != "" {
			continue
		}
		data, ok := fetch(h.Path)
		if !ok {
			h.ForgeNote = "no reachable forge (gh/glab) — PRs, claims, and CI unseen; git-only derivation"
			continue
		}
		h.Forge = data.Repo
		enrichRepoForge(res, repoCtx{name: h.Name, path: h.Path, base: h.Integration}, data, opts)
	}
}

// enrichRepoForge applies one repo's forge data to that repo's units,
// claims, and landings. Matching is keyed on the unit's repo — a spoke
// branch is only ever compared against its own forge's PRs; matching it
// against a hub PR of the same name would be a false claim.
func enrichRepoForge(res *Result, ctx repoCtx, data *forge.Data, opts Options) {
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
		if u.Repo != ctx.name {
			continue
		}
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
				Subject: u.Label(),
				Detail:  fmt.Sprintf("PR #%d was closed without merging but the branch still exists", pr.Number),
			})
		}
	}

	known := map[int]bool{}
	for _, issue := range data.Issues {
		known[issue.Number] = true
	}
	evidence := collectIssueEvidence(ctx, res.Units, unitHasTicket, known, opts)

	for _, issue := range data.Issues {
		if issue.State != "OPEN" {
			continue
		}
		ev := evidence.byIssue[issue.Number]
		// Claim subjects carry the repo the claim was checked in — "#12"
		// in the hub, "api:#12" in a spoke.
		subject := ctx.label(fmt.Sprintf("#%d", issue.Number))
		switch {
		case ev.fixesMerged:
			res.Claims = append(res.Claims, Claim{
				Kind:    "ticket-done-but-open",
				Subject: subject,
				Detail: fmt.Sprintf("%q: a fixing commit already landed on %s but the ticket is still open",
					issue.Title, ctx.label(ctx.base)),
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

	// CI verdict on landed specs: only a red signal changes anything; no
	// data or pending means saying nothing (honesty rule). A landing is
	// only ever checked against the CI of the repo it landed in.
	if data.Checks != nil {
		declared := ctx.name // the name repos: intent uses for this repo
		if declared == "" {
			declared = spec.HubRepo
		}
		for i := range res.Specs {
			ss := &res.Specs[i]
			if ss.Status != Done {
				continue
			}
			if len(ss.Repos) > 0 {
				// repos: specs land per repo; red CI on any declared
				// landing regresses the whole promise (tb-c512 semantics).
				for _, rl := range ss.PerRepo {
					if rl.Repo != declared || rl.State != "landed" {
						continue
					}
					if state, ok := data.Checks(rl.SHA); ok && state == "failure" {
						ss.Status = Regressed
						ss.Evidence = fmt.Sprintf("CI is red on landing commit %.7s in %s", rl.SHA, rl.Repo)
					}
				}
				continue
			}
			if ss.Landed == "" || ss.LandedRepo != ctx.name {
				continue
			}
			if state, ok := data.Checks(ss.Landed); ok && state == "failure" {
				ss.Status = Regressed
				ss.Evidence = fmt.Sprintf("CI is red on landing commit %.7s", ss.Landed)
				if ctx.name != "" {
					ss.Evidence += " in " + ctx.name
				}
			}
		}
	}

	for _, u := range res.Units {
		if u.Repo != ctx.name || u.Status == Done || u.SpecID != "" || unitHasTicket[u.Name] || evidence.unitRefs[u.Name] {
			continue
		}
		res.Claims = append(res.Claims, Claim{
			Kind:    "unticketed-work",
			Subject: u.Label(),
			Detail:  "no PR and no issue reference in any of its commits — work nobody promised",
		})
	}

	if ctx.name == "" {
		res.Forge = data.Repo
	}
}

type issueEvidence struct {
	byIssue  map[int]refEvidence
	unitRefs map[string]bool // unit branch mentions at least one issue
}

// collectIssueEvidence scans commit subjects+bodies for #N references: the
// integration branch within the digest window (merged proof) and each work
// branch's unmerged commits (intent proof) — in one repo, against that
// repo's own tracker. Only references to issues that actually exist in the
// tracker count — commit messages are full of incidental #N strings
// (milestones, PR numbers, "item #2").
func collectIssueEvidence(ctx repoCtx, units []Unit, skip map[string]bool, known map[int]bool, opts Options) issueEvidence {
	ev := issueEvidence{byIssue: map[int]refEvidence{}, unitRefs: map[string]bool{}}

	if out, ok := gitrepo.Try(ctx.path, "log", ctx.base,
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
		if u.Repo != ctx.name || u.Status == Done || skip[u.Name] {
			continue
		}
		out, ok := gitrepo.Try(ctx.path, "log", "-n", "200", ctx.base+".."+u.Tip, "--format=%s %b%x00")
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
