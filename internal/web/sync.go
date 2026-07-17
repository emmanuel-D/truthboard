// The sync loop is the one deliberate exception to the tool's read-only
// doctrine (and so lives here, not in gitrepo): a remote-watching board —
// a PO laptop, a shared box — must run `git fetch` itself or it silently
// drifts from repo reality. Proof (remote-tracking refs) is refreshed
// unconditionally; intent (spec files in the working tree) only when a
// fast-forward cannot possibly touch anyone's work.
package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// syncer keeps origin fresh and reports exactly how fresh it managed to be.
type syncer struct {
	repo  string
	every time.Duration

	mu       sync.Mutex
	stepping sync.Mutex // held while a webhook-kicked step runs, to coalesce storms
	at       time.Time  // last successful fetch
	err      string     // last fetch failure
	note     string     // why the working tree (intent) was left stale
}

func (s *syncer) run() {
	for {
		s.kick()
		time.Sleep(s.every)
	}
}

// kick runs one sync step now — the webhook's "a push just happened".
// Coalesced: a storm of pushes while a fetch is already running does not
// queue further fetches; the running one already sees the new commits.
func (s *syncer) kick() {
	if !s.stepping.TryLock() {
		return
	}
	defer s.stepping.Unlock()
	s.step()
}

// step fetches, then fast-forwards the checkout only when that is provably
// harmless: clean working tree, sitting on the integration branch.
func (s *syncer) step() {
	if _, err := gitMutate(s.repo, "fetch", "--prune", "--quiet", "origin"); err != nil {
		s.set(func() { s.err = oneLine(err.Error()) })
		return
	}
	s.set(func() { s.at, s.err = time.Now(), "" })
	s.set(func() { s.note = s.fastForward() })
}

// fastForward returns "" when intent is fresh, otherwise the reason it was
// left alone — never a guess, never a forced move.
func (s *syncer) fastForward() string {
	ref, ok := gitrepo.Try(s.repo, "symbolic-ref", "refs/remotes/origin/HEAD")
	if !ok {
		return "origin has no default branch recorded; working tree left alone"
	}
	integration := strings.TrimPrefix(ref, "refs/remotes/origin/")
	head, ok := gitrepo.Try(s.repo, "symbolic-ref", "--short", "HEAD")
	if !ok {
		return "detached HEAD; working tree left alone"
	}
	if head != integration {
		return fmt.Sprintf("checked out on %s, not %s; spec files reflect that branch", head, integration)
	}
	if out, ok := gitrepo.Try(s.repo, "--no-optional-locks", "status", "--porcelain"); !ok || out != "" {
		return "working tree has uncommitted changes; not fast-forwarded"
	}
	if _, err := gitMutate(s.repo, "merge", "--ff-only", "--quiet", "origin/"+integration); err != nil {
		return oneLine(fmt.Sprintf("%s cannot fast-forward to origin/%s: %v", integration, integration, err))
	}
	return ""
}

func (s *syncer) set(f func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f()
}

// headers reports sync freshness on every board response, so the page can
// never mistake a stale board for a quiet repo.
func (s *syncer) headers(h http.Header) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h.Set("X-Truthboard-Sync-Every", fmt.Sprint(int(s.every.Seconds())))
	if !s.at.IsZero() {
		h.Set("X-Truthboard-Sync-At", s.at.UTC().Format(time.RFC3339))
	}
	if s.err != "" {
		h.Set("X-Truthboard-Sync-Err", s.err)
	}
	if s.note != "" {
		h.Set("X-Truthboard-Sync-Note", s.note)
	}
}

// gitMutate runs a git command that is allowed to change repo state. It
// never lets git prompt for credentials (a background loop must fail
// loudly, not hang on a hidden password prompt) and gives up after a
// minute so one wedged fetch cannot stall the loop forever.
func gitMutate(repo string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(string(out)), nil
}

// oneLine collapses a multi-line git message into a header-safe sentence.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
