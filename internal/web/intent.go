// Landing intent from a shared board: a token-armed board is often the
// only writer on its machine — a phone-made story that stays an
// uncommitted file on some server helps no one. So each authenticated
// intent write is committed and pushed to origin, where every clone
// (and every agent running `truthboard next`) picks it up. Only the
// promise travels this path; statuses have no route here or anywhere.
package web

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// tokenOK accepts the edit token from X-Truthboard-Token or a bearer
// Authorization header, compared in constant time like the webhook secret.
func tokenOK(r *http.Request, token string) bool {
	got := r.Header.Get("X-Truthboard-Token")
	if got == "" {
		got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// committer lands one intent file per call: add, commit, rebase on
// whatever landed on origin meanwhile, push. Serialized — two phones
// editing at once queue here instead of interleaving git commands.
type committer struct {
	repo string
	mu   sync.Mutex
}

// land commits the spec file and pushes it to origin. The commit message
// deliberately carries the spec id bare, never as a `Spec:` trailer — a
// trailer on the integration branch would derive the story as done, and
// writing intent must not fabricate proof.
func (c *committer) land(file, subject string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := gitMutate(c.repo, "add", "--", file); err != nil {
		return err
	}
	// An edit that changed nothing (same owner typed again) has nothing
	// to land — git commit would fail loudly over a non-event.
	if _, ok := gitrepo.Try(c.repo, "diff", "--cached", "--quiet", "--", file); ok {
		return nil
	}
	if _, err := gitMutate(c.repo,
		"-c", "user.name=Truthboard shared board",
		"-c", "user.email=board@truthboard",
		"commit", "--quiet", "-m", subject, "--", file); err != nil {
		return err
	}
	branch, ok := gitrepo.Try(c.repo, "symbolic-ref", "--short", "HEAD")
	if !ok {
		return fmt.Errorf("the board's clone is on a detached HEAD — the edit is committed locally but cannot be pushed")
	}
	// Someone else's intent may have landed since the last fetch; rebase
	// ours on top (identity again: replaying commits needs a committer,
	// and the server clone may have none configured). On conflict, back
	// out cleanly — the specs on disk must never be left mid-rebase.
	if _, err := gitMutate(c.repo,
		"-c", "user.name=Truthboard shared board",
		"-c", "user.email=board@truthboard",
		"pull", "--rebase", "--autostash", "--quiet", "origin", branch); err != nil {
		gitMutate(c.repo, "rebase", "--abort")
		return fmt.Errorf("conflicts with an edit that just landed on origin: %v — the edit is committed on the board's clone; resolve from a clone with push access", err)
	}
	if _, err := gitMutate(c.repo, "push", "--quiet", "origin", branch); err != nil {
		return fmt.Errorf("committed on the board's clone but pushing to origin failed: %v — check the clone's push credentials", err)
	}
	return nil
}
