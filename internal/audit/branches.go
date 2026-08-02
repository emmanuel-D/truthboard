package audit

import (
	"fmt"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// BranchTarget is one branch seen from the angle of retiring it: which refs
// exist, whether git already holds its work, and the two conditions that
// make deletion impossible rather than merely unwise.
//
// It answers, and never acts. Deleting is a mutation, and this package
// stays read-only — the caller that owns the mutation (the board server)
// decides what to do with these facts.
type BranchTarget struct {
	Repo        string `json:"repo,omitempty"` // workspace spoke; empty means the hub
	Name        string `json:"branch"`
	Path        string `json:"-"` // repository the refs live in
	Integration string `json:"integration"`

	Local  bool `json:"local"`  // a local ref exists
	Remote bool `json:"remote"` // origin has it
	// Bare marks a managed mirror clone, where refs/heads is the mirror OF
	// origin rather than anyone's local work: there is no local branch to
	// delete, only origin's — plus the mirror's stale copy of it.
	Bare bool `json:"-"`

	IsIntegration bool   `json:"is_integration"`
	CheckedOut    bool   `json:"checked_out"`
	Merged        bool   `json:"merged"`
	Ahead         int    `json:"ahead"`
	Evidence      string `json:"evidence"`
}

// Label names the branch the way the board does — repo-prefixed in a
// workspace spoke.
func (t BranchTarget) Label() string {
	if t.Repo == "" {
		return t.Name
	}
	return t.Repo + ":" + t.Name
}

// FindBranch resolves one branch in the hub or in a named workspace spoke
// and reports everything a caller needs to decide whether deleting it is
// safe. repoName is empty or "hub" for the hub itself.
func FindBranch(hub, repoName, branch string) (*BranchTarget, error) {
	ctx, err := resolveRepo(hub, repoName)
	if err != nil {
		return nil, err
	}
	branches, err := collectBranches(ctx.path)
	if err != nil {
		return nil, err
	}
	tip, ok := branches[branch]
	if !ok {
		where := "this repository"
		if ctx.name != "" {
			where = "workspace repo " + ctx.name
		}
		return nil, fmt.Errorf("no branch %q in %s", branch, where)
	}

	u := classify(ctx.path, ctx.base, branch, tip, Options{}.normalized())
	t := &BranchTarget{
		Repo:        ctx.name,
		Name:        branch,
		Path:        ctx.path,
		Integration: ctx.base,
		Local:       tip.local,
		Remote:      tip.remote,
		Merged:      u.Status == Done,
		Ahead:       u.Ahead,
		Evidence:    u.Evidence,
	}
	// A branch whose tip IS the integration tip never reaches the board as
	// a unit (nothing of its own has happened on it), so classify calls it
	// merged — which is true, and is exactly why deleting it loses nothing.

	elected := strings.TrimPrefix(ctx.base, "origin/")
	t.IsIntegration = integrationNames[branch] || branch == elected
	if head, ok := gitrepo.Try(ctx.path, "symbolic-ref", "--short", "HEAD"); ok && head == branch {
		t.CheckedOut = true
	}
	if out, ok := gitrepo.Try(ctx.path, "rev-parse", "--is-bare-repository"); ok && out == "true" {
		t.Bare, t.Local, t.Remote = true, false, true
	}
	return t, nil
}

// resolveRepo opens the hub or one declared spoke as an auditable context.
func resolveRepo(hub, name string) (repoCtx, error) {
	if name == "" || name == "hub" {
		branches, err := collectBranches(hub)
		if err != nil {
			return repoCtx{}, err
		}
		elected, _, _, err := electIntegration(hub, branches)
		if err != nil {
			return repoCtx{}, err
		}
		return repoCtx{path: hub, base: integrationRef(hub, elected)}, nil
	}
	ws, err := workspace.Load(hub)
	if err != nil {
		return repoCtx{}, err
	}
	if ws == nil {
		return repoCtx{}, fmt.Errorf("%q is not a repository this board serves — there is no workspace here", name)
	}
	for _, r := range ws.Repos {
		if r.Name == name {
			return openSpoke(ws, r)
		}
	}
	return repoCtx{}, fmt.Errorf("%q is not a repository this workspace declares", name)
}
