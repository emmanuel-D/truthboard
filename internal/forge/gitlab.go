package forge

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// GitLab REST shapes as returned by `glab api` (raw API JSON, stable across
// glab versions — unlike the list subcommands' output flags).
type glabMR struct {
	IID          int       `json:"iid"`
	Title        string    `json:"title"`
	State        string    `json:"state"` // opened, merged, closed, locked
	Draft        bool      `json:"draft"`
	SourceBranch string    `json:"source_branch"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type glabIssue struct {
	IID       int       `json:"iid"`
	Title     string    `json:"title"`
	State     string    `json:"state"` // opened, closed
	UpdatedAt time.Time `json:"updated_at"`
	Assignees []struct {
		Username string `json:"username"`
	} `json:"assignees"`
}

// FetchGitLab returns tracker data for the repository at path via glab, or
// ok=false when glab is missing, unauthenticated, or the repo isn't on a
// GitLab instance. Failures are silent by contract.
func FetchGitLab(path string) (*Data, bool) {
	if _, err := exec.LookPath("glab"); err != nil {
		return nil, false
	}
	mrsRaw, mrsOK := runIn(path, "glab", "api", "projects/:id/merge_requests?state=all&per_page=100")
	issuesRaw, issuesOK := runIn(path, "glab", "api", "projects/:id/issues?per_page=100")
	if !mrsOK && !issuesOK {
		return nil, false
	}
	data := &Data{Repo: gitlabRepoName(path)}
	if mrsOK {
		var mrs []glabMR
		if json.Unmarshal(mrsRaw, &mrs) == nil {
			data.PRs = convertGitLabMRs(mrs)
		}
	}
	if issuesOK {
		var issues []glabIssue
		if json.Unmarshal(issuesRaw, &issues) == nil {
			data.Issues = convertGitLabIssues(issues)
		}
	}
	data.Checks = func(sha string) (string, bool) { return gitlabChecks(path, sha) }
	return data, true
}

// gitlabChecks maps the commit's last pipeline status to the shared CI
// vocabulary; commits with no pipeline are honestly "unknown".
func gitlabChecks(path, sha string) (string, bool) {
	raw, ok := runIn(path, "glab", "api", "projects/:id/repository/commits/"+sha)
	if !ok {
		return "", false
	}
	var resp struct {
		LastPipeline *struct {
			Status string `json:"status"`
		} `json:"last_pipeline"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return "", false
	}
	if resp.LastPipeline == nil {
		return "unknown", true
	}
	switch resp.LastPipeline.Status {
	case "failed":
		return "failure", true
	case "success":
		return "success", true
	case "running", "pending", "created":
		return "pending", true
	}
	return "unknown", true
}

// convertGitLabMRs normalizes MRs to the shared PR shape so the claims
// engine treats both forges identically.
func convertGitLabMRs(mrs []glabMR) []PR {
	prs := make([]PR, 0, len(mrs))
	for _, mr := range mrs {
		state := "OPEN"
		switch mr.State {
		case "merged":
			state = "MERGED"
		case "closed":
			state = "CLOSED"
		}
		prs = append(prs, PR{
			Number:      mr.IID,
			Title:       mr.Title,
			State:       state,
			IsDraft:     mr.Draft,
			HeadRefName: mr.SourceBranch,
			UpdatedAt:   mr.UpdatedAt,
		})
	}
	return prs
}

func convertGitLabIssues(issues []glabIssue) []Issue {
	out := make([]Issue, 0, len(issues))
	for _, is := range issues {
		state := "CLOSED"
		if is.State == "opened" {
			state = "OPEN"
		}
		conv := Issue{Number: is.IID, Title: is.Title, State: state, UpdatedAt: is.UpdatedAt}
		for _, a := range is.Assignees {
			conv.Assignees = append(conv.Assignees, Assignee{Login: a.Username})
		}
		out = append(out, conv)
	}
	return out
}

var remotePathPattern = regexp.MustCompile(`^(?:[a-z+]+://[^/]+/|[^@]+@[^:]+:)(.+?)(?:\.git)?$`)

// gitlabRepoName derives namespace/project from the origin URL; glab has no
// stable cross-version "repo view --json", so parse it ourselves.
func gitlabRepoName(path string) string {
	cmd := exec.Command("git", "-C", path, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	if m := remotePathPattern.FindStringSubmatch(strings.TrimSpace(string(out))); m != nil {
		return m[1]
	}
	return ""
}

func runIn(dir, name string, args ...string) ([]byte, bool) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return out, err == nil
}
