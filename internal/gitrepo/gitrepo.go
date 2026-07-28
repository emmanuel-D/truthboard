// Package gitrepo is a thin runner for git plumbing commands against a
// repository path. The audit engine is read-only by design, so nothing in
// this package may ever mutate the target repo.
package gitrepo

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// credentialInURL matches the userinfo of a URL — the "user:token@" that
// the documented way of reaching a private repo puts there. Anchored on a
// scheme so scp-style remotes (git@host:path, which carry no secret) are
// left alone, and stopped at "/" so an @ inside a path is not userinfo.
var credentialInURL = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s@]+@`)

// Redact replaces embedded credentials in any text carrying a remote URL.
//
// It lives here because this is where git output is born. A token reaches
// a log by being handed to git as an argument and echoed back in an error,
// and the reader of that error is usually a shared board serving it to
// anyone who can load the page.
func Redact(s string) string {
	return credentialInURL.ReplaceAllString(s, "${1}***@")
}

// Run executes a git command in repo and returns trimmed stdout, failing
// with stderr context on a non-zero exit.
//
// Both halves of that failure are redacted. The arguments because a clone
// takes its URL there — credentials included — and the message because git
// sanitizes most of its own output but not all of it, and "most" is not a
// property worth betting a token on.
func Run(repo string, args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(msg, "not a git repository") {
			return "", notARepoError(repo)
		}
		return "", fmt.Errorf("git %s: %s", Redact(strings.Join(args, " ")), Redact(msg))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// notARepoError replaces git's plumbing complaint with the one thing the
// reader can act on. This is the first error a fresh hub hits — a directory
// scaffolded by hand and never `git init`ed — and naming the internal
// for-each-ref invocation there tells them nothing.
func notARepoError(repo string) error {
	abs, err := filepath.Abs(repo)
	if err != nil {
		abs = repo
	}
	return fmt.Errorf("%s is not a git repository — truthboard derives every "+
		"status from branches, merges and commit trailers, so there is nothing "+
		"to read yet; run \"git init\" there first", abs)
}

// Try executes a git command where a non-zero exit is an expected answer
// (e.g. merge-base --is-ancestor), not an error.
func Try(repo string, args ...string) (string, bool) {
	out, err := Run(repo, args...)
	return out, err == nil
}
