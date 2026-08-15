package mirror

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// Issue is the little of a forge issue mirroring needs: enough to tell
// whether the copy still matches the story.
type Issue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"` // open | closed
	Body   string `json:"body"`
}

// Client is the forge, narrowed to the four things mirroring does. It is an
// interface so the plan, the rendering and every decision above can be
// tested without a network, an account, or somebody's real issue tracker.
type Client interface {
	Repo() string
	List() ([]Issue, error)
	Create(title, body string) (int, error)
	Update(number int, title, body string) error
	Close(number int) error
}

// Open finds the forge for a repository — GitHub via gh, GitLab via glab —
// and fails loudly when it cannot.
//
// Loudly is the point, and it is the opposite of what forge enrichment
// does. Enrichment degrades in silence because the audit must keep working
// without a forge; mirroring is a thing somebody asked for, and a mirror
// command that shrugged and exited zero would leave a person believing
// their board was published when it was not.
func Open(repo string) (Client, error) {
	if _, err := exec.LookPath("gh"); err == nil {
		out, err := run(repo, "gh", "repo", "view", "--json", "nameWithOwner")
		if err == nil {
			var v struct {
				NameWithOwner string `json:"nameWithOwner"`
			}
			if json.Unmarshal(out, &v) == nil && v.NameWithOwner != "" {
				return &ghClient{repo: repo, name: v.NameWithOwner}, nil
			}
		} else if isAuthError(err) {
			return nil, fmt.Errorf("gh is installed but not authenticated — run `gh auth login`: %w", err)
		}
	}
	if _, err := exec.LookPath("glab"); err == nil {
		out, err := run(repo, "glab", "repo", "view", "-F", "json")
		if err == nil {
			var v struct {
				PathWithNamespace string `json:"path_with_namespace"`
			}
			if json.Unmarshal(out, &v) == nil && v.PathWithNamespace != "" {
				return &glabClient{repo: repo, name: v.PathWithNamespace}, nil
			}
		} else if isAuthError(err) {
			return nil, fmt.Errorf("glab is installed but not authenticated — run `glab auth login`: %w", err)
		}
	}
	return nil, fmt.Errorf("no forge reachable from %s — mirroring needs `gh` (GitHub) or `glab` (GitLab), "+
		"installed, authenticated, and a remote pointing at the repository", repo)
}

// run executes a forge CLI and turns a failure into an error that names the
// cause. Both halves are redacted: a token can reach a command line and an
// error quotes what it ran.
func run(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err == nil {
		return out, nil
	}
	msg := err.Error()
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		msg = strings.TrimSpace(string(ee.Stderr))
	}
	return nil, fmt.Errorf("%s %s: %s", name, args[0], gitrepo.Redact(msg))
}

// isAuthError recognises the failures worth naming: nobody is helped by
// "exit status 1" when the fix is one login away.
func isAuthError(err error) bool {
	s := strings.ToLower(err.Error())
	for _, hint := range []string{"auth", "login", "credential", "token", "unauthorized", "401", "403"} {
		if strings.Contains(s, hint) {
			return true
		}
	}
	return false
}

type ghClient struct {
	repo string
	name string
}

func (c *ghClient) Repo() string { return c.name }

func (c *ghClient) List() ([]Issue, error) {
	out, err := run(c.repo, "gh", "issue", "list", "--state", "all", "--limit", "500",
		"--json", "number,title,state,body")
	if err != nil {
		return nil, err
	}
	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("reading issues from gh: %w", err)
	}
	// gh reports OPEN/CLOSED; the plan compares against lowercase.
	for i := range issues {
		issues[i].State = strings.ToLower(issues[i].State)
	}
	return issues, nil
}

func (c *ghClient) Create(title, body string) (int, error) {
	out, err := run(c.repo, "gh", "issue", "create", "--title", title, "--body", body)
	if err != nil {
		return 0, err
	}
	return issueNumberFromURL(string(out)), nil
}

func (c *ghClient) Update(number int, title, body string) error {
	_, err := run(c.repo, "gh", "issue", "edit", strconv.Itoa(number), "--title", title, "--body", body)
	return err
}

func (c *ghClient) Close(number int) error {
	_, err := run(c.repo, "gh", "issue", "close", strconv.Itoa(number),
		"--comment", "The story this mirrors derived done from git.")
	return err
}

type glabClient struct {
	repo string
	name string
}

func (c *glabClient) Repo() string { return c.name }

func (c *glabClient) List() ([]Issue, error) {
	out, err := run(c.repo, "glab", "issue", "list", "--all", "--per-page", "500", "-F", "json")
	if err != nil {
		return nil, err
	}
	var raw []struct {
		IID   int    `json:"iid"`
		Title string `json:"title"`
		State string `json:"state"`
		Desc  string `json:"description"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("reading issues from glab: %w", err)
	}
	issues := make([]Issue, 0, len(raw))
	for _, r := range raw {
		state := strings.ToLower(r.State)
		if state == "opened" {
			state = "open"
		}
		issues = append(issues, Issue{Number: r.IID, Title: r.Title, State: state, Body: r.Desc})
	}
	return issues, nil
}

func (c *glabClient) Create(title, body string) (int, error) {
	out, err := run(c.repo, "glab", "issue", "create", "--title", title, "--description", body, "--yes")
	if err != nil {
		return 0, err
	}
	return issueNumberFromURL(string(out)), nil
}

func (c *glabClient) Update(number int, title, body string) error {
	_, err := run(c.repo, "glab", "issue", "update", strconv.Itoa(number),
		"--title", title, "--description", body)
	return err
}

func (c *glabClient) Close(number int) error {
	_, err := run(c.repo, "glab", "issue", "close", strconv.Itoa(number))
	return err
}

var trailingNumber = regexp.MustCompile(`/(\d+)\s*$`)

// issueNumberFromURL reads the new issue's number out of the URL both CLIs
// print. Zero when it cannot be read, which costs nothing: the next run
// finds the issue by its title like every other one.
func issueNumberFromURL(out string) int {
	m := trailingNumber.FindStringSubmatch(strings.TrimSpace(out))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}
