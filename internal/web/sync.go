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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// syncer keeps origin fresh and reports exactly how fresh it managed to be.
type syncer struct {
	repo  string
	every time.Duration

	// Workspace spokes: name labels this repo in headers, remoteURL arms
	// self-healing (a missing clone dir is mirror-cloned on the next step),
	// and proofOnly skips the working-tree fast-forward — a mirror has no
	// working tree, and a declared spoke checkout is someone else's; proof
	// (refs) is all the board needs from either.
	name      string
	remoteURL string
	proofOnly bool

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
// harmless: clean working tree, sitting on the integration branch. A spoke
// whose managed clone does not exist yet is cloned first — the sync loop is
// the one place allowed to mutate, so this is where clones are born.
func (s *syncer) step() {
	if s.remoteURL != "" {
		if _, statErr := os.Stat(s.repo); statErr != nil {
			if err := os.MkdirAll(filepath.Dir(s.repo), 0o755); err != nil {
				s.set(func() { s.err = oneLine(err.Error()) })
				return
			}
			if _, err := gitMutate(filepath.Dir(s.repo), "clone", "--mirror", s.remoteURL, s.repo); err != nil {
				s.set(func() { s.err = oneLine(err.Error()) })
				return
			}
		}
	}
	if _, err := gitMutate(s.repo, "fetch", "--prune", "--quiet", "origin"); err != nil {
		s.set(func() { s.err = oneLine(err.Error()) })
		return
	}
	s.set(func() { s.at, s.err = time.Now(), "" })
	if s.proofOnly {
		s.set(func() { s.note = "" })
		return
	}
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

// syncGroup fans the sync loop out over a workspace: the hub plus one
// syncer per declared spoke. With no manifest it is a group of one and
// behaves exactly like the single syncer always has.
type syncGroup struct {
	members []*syncer
}

// newSyncGroup builds the hub syncer plus one per resolvable spoke. A spoke
// with a live local checkout is fetched in place; one with only a remote
// gets a managed mirror clone under the hub's git dir, created lazily by
// its own sync step. Manifest errors are not swallowed here so much as
// deferred: the audit reports them on the board itself.
func newSyncGroup(hub string, every time.Duration) *syncGroup {
	g := &syncGroup{members: []*syncer{{repo: hub, every: every}}}
	ws, err := workspace.Load(hub)
	if err != nil || ws == nil {
		return g
	}
	for _, r := range ws.Repos {
		s := &syncer{name: r.Name, every: every, proofOnly: true}
		if path, err := ws.Resolve(r); err == nil && path != workspace.CloneDir(hub, r.Name) {
			s.repo = path // declared checkout: fetch it, never touch its tree
		} else {
			if r.Remote == "" {
				continue // path-only spoke that doesn't exist: audit reports it
			}
			s.repo, s.remoteURL = workspace.CloneDir(hub, r.Name), r.Remote
		}
		g.members = append(g.members, s)
	}
	return g
}

func (g *syncGroup) run() {
	for _, m := range g.members {
		go m.run()
	}
}

func (g *syncGroup) kick() {
	for _, m := range g.members {
		m.kick()
	}
}

// headers reports sync freshness for the whole workspace. The At header is
// the OLDEST member fetch — and is omitted entirely while any member has
// never fetched — so a stale or missing spoke can never hide behind a fresh
// hub. Errors and notes carry the spoke's name.
func (g *syncGroup) headers(h http.Header) {
	if len(g.members) == 1 {
		g.members[0].headers(h)
		return
	}
	h.Set("X-Truthboard-Sync-Every", fmt.Sprint(int(g.members[0].every.Seconds())))
	oldest := time.Time{}
	allFetched := true
	var errs, notes []string
	for _, m := range g.members {
		m.mu.Lock()
		at, err, note := m.at, m.err, m.note
		m.mu.Unlock()
		label := func(s string) string {
			if m.name == "" {
				return s
			}
			return m.name + ": " + s
		}
		if at.IsZero() {
			allFetched = false
			if err == "" {
				notes = append(notes, label("not fetched yet"))
			}
		} else if oldest.IsZero() || at.Before(oldest) {
			oldest = at
		}
		if err != "" {
			errs = append(errs, label(err))
		}
		if note != "" {
			notes = append(notes, label(note))
		}
	}
	if allFetched && !oldest.IsZero() {
		h.Set("X-Truthboard-Sync-At", oldest.UTC().Format(time.RFC3339))
	}
	if len(errs) > 0 {
		h.Set("X-Truthboard-Sync-Err", strings.Join(errs, "; "))
	}
	if len(notes) > 0 {
		h.Set("X-Truthboard-Sync-Note", strings.Join(notes, "; "))
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
