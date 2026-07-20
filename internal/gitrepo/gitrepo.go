// Package gitrepo is a thin runner for git plumbing commands against a
// repository path. The audit engine is read-only by design, so nothing in
// this package may ever mutate the target repo.
package gitrepo

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes a git command in repo and returns trimmed stdout, failing
// with stderr context on a non-zero exit.
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
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
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
