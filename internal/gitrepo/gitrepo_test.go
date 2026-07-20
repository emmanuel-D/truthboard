package gitrepo

import (
	"os/exec"
	"strings"
	"testing"
)

// The plumbing failure a fresh hub hits first must read as guidance, not as
// the name of an internal git invocation.
func TestRunTranslatesNotARepository(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(dir, "for-each-ref", "refs/heads")
	if err == nil {
		t.Fatal("Run() in a plain directory = nil error, want a failure")
	}
	msg := err.Error()
	for _, want := range []string{dir, "not a git repository", `run "git init"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("error missing %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "for-each-ref") {
		t.Errorf("error leaks the plumbing command: %s", msg)
	}
}

// Every other git failure keeps its command context — that detail is the
// useful part when the repo is real and the command is wrong.
func TestRunKeepsCommandContextOnOtherFailures(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	_, err := Run(dir, "rev-parse", "refs/heads/nope")
	if err == nil {
		t.Fatal("Run() on a missing ref = nil error, want a failure")
	}
	if !strings.Contains(err.Error(), "rev-parse") {
		t.Errorf("error dropped the command: %s", err)
	}
}
