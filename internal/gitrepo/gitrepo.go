// Package gitrepo is a thin runner for git plumbing commands against a
// repository path. The audit engine is read-only by design, so nothing in
// this package may ever mutate the target repo.
package gitrepo

import (
	"bytes"
	"fmt"
	"os/exec"
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
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Try executes a git command where a non-zero exit is an expected answer
// (e.g. merge-base --is-ancestor), not an error.
func Try(repo string, args ...string) (string, bool) {
	out, err := Run(repo, args...)
	return out, err == nil
}
