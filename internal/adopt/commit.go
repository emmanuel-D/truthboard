package adopt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// CommitMessage is the subject of the commit that adopts truthboard. It is
// the same wording RepoWarning tells a hand-adopter to use, so a workspace
// reads the same however it was set up.
const CommitMessage = "Track work with truthboard"

// governedPaths are the files this commit may contain. Exactly the fileset
// the audit exempts from shadow work (tb-3d43) and the commit-msg nudge
// stays quiet about: a commit confined to them changes how work is tracked,
// never the product. Staging by explicit path — never `git add -A` — is what
// keeps that true in a checkout with unrelated edits in it.
var governedPaths = []string{".truthboard", ".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md"}

// Commit records the wiring in the repository it was written to.
//
// Authoring this on someone's behalf is only defensible because the board
// already refuses to hold it against them: tb-3d43 exempted exactly this
// fileset from shadow work, after a real hub's own adoption commit was
// reported as drift on the board it created. It has to land directly on the
// integration branch — there is no board to open a PR against yet — which is
// precisely the shape shadow-work detection looks for.
//
// Returns a log line describing what happened. Nothing to stage is a no-op,
// not an error: re-running an adopted workspace should say so and stop.
func Commit(repo string) (string, error) {
	present := false
	for _, p := range governedPaths {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(p))); err != nil {
			continue
		}
		if _, err := gitrepo.Run(repo, "add", "--", p); err != nil {
			return "", err
		}
		present = true
	}
	if !present {
		return "nothing to commit — no wiring on disk", nil
	}
	// The pathspec is the files git actually staged, not the paths asked
	// for. `.truthboard` is a directory and may hold nothing yet — a hub
	// whose specs are still to be written — and naming an empty directory as
	// a pathspec is an error, not an empty commit.
	//
	// --cached compares against HEAD, so an unborn branch (the repository
	// `git init` made moments ago) reports its staged files rather than
	// failing.
	diff, err := gitrepo.Run(repo, "diff", "--cached", "--name-only")
	if err != nil {
		return "", err
	}
	staged := governedOnly(strings.Split(diff, "\n"))
	if len(staged) == 0 {
		return "already committed — nothing changed", nil
	}
	// Pathspec-limited to those files, so unrelated work already sitting in
	// the index is never swept into a commit the adopter did not write —
	// which would be a mixed commit, i.e. shadow work, authored by us.
	commit := append([]string{"commit", "-m", CommitMessage, "--"}, staged...)
	if _, err := gitrepo.Run(repo, commit...); err != nil {
		return "", fmt.Errorf("%w\n  (a commit needs user.name and user.email — set them, then re-run with --commit)", err)
	}
	head, _ := gitrepo.Try(repo, "rev-parse", "--short", "HEAD")
	return fmt.Sprintf("committed %s as %s", strings.Join(staged, ", "), head), nil
}

// governedOnly keeps the staged paths this commit owns.
func governedOnly(paths []string) []string {
	var kept []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for _, g := range governedPaths {
			if p == g || strings.HasPrefix(p, g+"/") {
				kept = append(kept, p)
				break
			}
		}
	}
	return kept
}

// CommitHint is what to print when --commit was not given: the wiring is
// intent and belongs in git, and a workspace whose manifest lives only on
// one laptop is a workspace nobody else can read.
func CommitHint(repos []string) []string {
	if len(repos) == 1 {
		return []string{fmt.Sprintf("this wiring is intent — commit it like code (or re-run with --commit):\n"+
			"    git add %s && git commit -m %q", strings.Join(governedPaths, " "), CommitMessage)}
	}
	return []string{fmt.Sprintf("this wiring is intent, in %d repositories — commit each like code,", len(repos)),
		"  or re-run with --commit to have truthboard do it"}
}
