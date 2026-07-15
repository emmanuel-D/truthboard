// Package forge fetches tracker claims (issues, PRs) from a forge via its
// CLI. Fetching is strictly read-only and strictly optional: when no forge
// is reachable the audit degrades to pure git inference.
package forge

import (
	"encoding/json"
	"os/exec"
	"time"
)

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"` // OPEN, CLOSED
	UpdatedAt time.Time `json:"updatedAt"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
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
	return data, true
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
