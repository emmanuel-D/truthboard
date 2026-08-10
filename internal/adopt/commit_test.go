package adopt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// identifiedRepo is a repository a commit can actually be made in.
func identifiedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "config", "user.name", "Test")
	gitIn(t, dir, "config", "user.email", "test@example.com")
	return dir
}

func TestCommitRecordsTheWiring(t *testing.T) {
	repo := identifiedRepo(t)
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	msg, err := Commit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "committed") {
		t.Errorf("log line = %q, want it to say what landed", msg)
	}
	if subject := gitIn(t, repo, "log", "-1", "--format=%s"); subject != CommitMessage {
		t.Errorf("subject = %q, want %q", subject, CommitMessage)
	}
	// Confined to governed files, which is what keeps this commit exempt
	// from the shadow-work finding it would otherwise become (tb-3d43).
	for _, f := range strings.Split(gitIn(t, repo, "show", "--name-only", "--format=", "HEAD"), "\n") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.HasPrefix(f, ".truthboard/") && f != ".mcp.json" && f != ".vscode/mcp.json" && f != "AGENTS.md" && f != "CLAUDE.md" {
			t.Errorf("commit carries an ungoverned file: %s", f)
		}
	}
}

// Product code sitting in the index is not this commit's to take: a mixed
// commit is shadow work, and truthboard would have authored it.
func TestCommitLeavesUnrelatedStagedWorkAlone(t *testing.T) {
	repo := identifiedRepo(t)
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "main.go")

	if _, err := Commit(repo); err != nil {
		t.Fatal(err)
	}
	if files := gitIn(t, repo, "show", "--name-only", "--format=", "HEAD"); strings.Contains(files, "main.go") {
		t.Errorf("the adoption commit swept up staged product code:\n%s", files)
	}
	if staged := gitIn(t, repo, "diff", "--cached", "--name-only"); staged != "main.go" {
		t.Errorf("staged work must survive, got %q", staged)
	}
}

func TestCommitIsANoOpOnAnAlreadyAdoptedRepo(t *testing.T) {
	repo := identifiedRepo(t)
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Commit(repo); err != nil {
		t.Fatal(err)
	}
	before := gitIn(t, repo, "rev-parse", "HEAD")

	msg, err := Commit(repo)
	if err != nil {
		t.Fatal(err)
	}
	if gitIn(t, repo, "rev-parse", "HEAD") != before {
		t.Error("a second run must not write an empty commit")
	}
	if !strings.Contains(msg, "already committed") {
		t.Errorf("log line = %q, want it to say nothing changed", msg)
	}
}

func TestCommitHintNamesTheFlagAndNeverRuns(t *testing.T) {
	repo := identifiedRepo(t)
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	hint := strings.Join(CommitHint([]string{repo}), "\n")
	if !strings.Contains(hint, "--commit") {
		t.Errorf("hint must name the flag that does it:\n%s", hint)
	}
	if n := gitIn(t, repo, "rev-list", "--count", "--all"); n != "0" {
		t.Errorf("printing a hint must not commit anything, got %s commits", n)
	}
}
