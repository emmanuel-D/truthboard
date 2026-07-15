// Package forge fetches tracker claims (issues, PRs) from a forge via its
// CLI. Fetching is strictly read-only and strictly optional: when no forge
// is reachable the audit degrades to pure git inference.
package forge

import (
	"encoding/json"
	"os/exec"
	"time"
)

type Assignee struct {
	Login string `json:"login"`
}

type Issue struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"` // OPEN, CLOSED
	UpdatedAt time.Time  `json:"updatedAt"`
	Assignees []Assignee `json:"assignees"`
}

// Assigned reports whether anyone has claimed the issue. An unassigned open
// issue is backlog, not a promise, and is never audited for staleness.
func (i Issue) Assigned() bool { return len(i.Assignees) > 0 }

type PR struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	State       string    `json:"state"` // OPEN, CLOSED, MERGED
	IsDraft     bool      `json:"isDraft"`
	HeadRefName string    `json:"headRefName"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type Data struct {
	Repo   string  `json:"repo"`
	Issues []Issue `json:"issues"`
	PRs    []PR    `json:"prs"`

	// Checks reports CI state ("success", "failure", "pending", "unknown")
	// for a commit, or ok=false when the forge can't answer. Lazy per-SHA
	// call — only consulted for landed specs. Nil when unavailable, and the
	// audit must then say nothing about CI rather than guess.
	Checks func(sha string) (string, bool) `json:"-"`
}

// Fetch tries each supported forge in turn: GitHub via gh, then GitLab via
// glab. Returns ok=false when no forge is reachable — the audit degrades to
// pure git inference.
func Fetch(path string) (*Data, bool) {
	if data, ok := FetchGitHub(path); ok {
		return data, true
	}
	return FetchGitLab(path)
}

// FetchGitHub returns tracker data for the repository at path, or ok=false
// when gh is not installed, not authenticated, or the repo has no GitHub
// remote. Failures are silent by contract — the audit must keep working.
func FetchGitHub(path string) (*Data, bool) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, false
	}
	repoName, ok := ghJSON[struct {
		NameWithOwner string `json:"nameWithOwner"`
	}](path, "repo", "view", "--json", "nameWithOwner")
	if !ok {
		return nil, false
	}
	data := &Data{Repo: repoName.NameWithOwner}
	if issues, ok := ghJSON[[]Issue](path, "issue", "list", "--state", "all", "--limit", "200",
		"--json", "number,title,state,updatedAt,assignees"); ok {
		data.Issues = issues
	}
	if prs, ok := ghJSON[[]PR](path, "pr", "list", "--state", "all", "--limit", "200",
		"--json", "number,title,state,isDraft,headRefName,updatedAt"); ok {
		data.PRs = prs
	}
	data.Checks = func(sha string) (string, bool) {
		return githubChecks(path, repoName.NameWithOwner, sha)
	}
	return data, true
}

// githubChecks aggregates check-run conclusions for a commit; any failed or
// timed-out run makes the whole commit red.
func githubChecks(path, nwo, sha string) (string, bool) {
	type checkRuns struct {
		TotalCount int `json:"total_count"`
		CheckRuns  []struct {
			Conclusion string `json:"conclusion"`
		} `json:"check_runs"`
	}
	cr, ok := ghJSON[checkRuns](path, "api", "repos/"+nwo+"/commits/"+sha+"/check-runs")
	if !ok {
		return "", false
	}
	if cr.TotalCount == 0 {
		return "unknown", true
	}
	state := "success"
	for _, r := range cr.CheckRuns {
		switch r.Conclusion {
		case "failure", "timed_out":
			return "failure", true
		case "": // still running
			state = "pending"
		}
	}
	return state, true
}

func ghJSON[T any](dir string, args ...string) (T, bool) {
	var zero T
	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return zero, false
	}
	var v T
	if err := json.Unmarshal(out, &v); err != nil {
		return zero, false
	}
	return v, true
}
