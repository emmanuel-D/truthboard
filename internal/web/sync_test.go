package web

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-m", msg)
}

// remoteFixture builds a bare origin with two clones: a writer (another
// machine pushing work) and a reader (the machine running the board).
func remoteFixture(t *testing.T) (writer, reader string) {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}
	clone := func(name string) string {
		dir := filepath.Join(t.TempDir(), name)
		if out, err := exec.Command("git", "clone", origin, dir).CombinedOutput(); err != nil {
			t.Fatalf("git clone: %v\n%s", err, out)
		}
		git(t, dir, "config", "user.email", "t@t.co")
		git(t, dir, "config", "user.name", "T")
		git(t, dir, "config", "commit.gpgsign", "false")
		return dir
	}
	writer = clone("writer")
	commitFile(t, writer, "README.md", "hi", "chore: init")
	git(t, writer, "push", "-u", "origin", "main")
	reader = clone("reader")
	// The reader clone must know origin's default branch, like a real clone.
	git(t, reader, "remote", "set-head", "origin", "main")
	return writer, reader
}

func TestSyncTracksRemoteWithoutLocalGitUse(t *testing.T) {
	writer, reader := remoteFixture(t)
	s := &syncer{repo: reader, every: time.Minute}

	// Another machine pushes a branch and new intent on main.
	git(t, writer, "checkout", "-b", "feature/tb-t1-demo")
	commitFile(t, writer, "work.txt", "wip", "feat: start\n\nSpec: tb-t1")
	git(t, writer, "push", "origin", "feature/tb-t1-demo")
	git(t, writer, "checkout", "main")
	commitFile(t, writer, "SPEC.md", "story", "story on main")
	git(t, writer, "push", "origin", "main")

	s.step()

	if _, err := exec.Command("git", "-C", reader, "rev-parse", "origin/feature/tb-t1-demo").CombinedOutput(); err != nil {
		t.Error("remote branch not visible after sync; fetch did not happen")
	}
	// Clean checkout on main: intent must have fast-forwarded too.
	if _, err := os.Stat(filepath.Join(reader, "SPEC.md")); err != nil {
		t.Errorf("working tree not fast-forwarded: %v", err)
	}
	if s.note != "" || s.err != "" {
		t.Errorf("clean sync reported note=%q err=%q, want none", s.note, s.err)
	}
	if s.at.IsZero() {
		t.Error("successful fetch must record its time")
	}
}

func TestSyncNeverTouchesDirtyOrDivergedWork(t *testing.T) {
	writer, reader := remoteFixture(t)
	s := &syncer{repo: reader, every: time.Minute}

	// Dirty working tree: refs refresh, the checkout stays untouched.
	if err := os.WriteFile(filepath.Join(reader, "local.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitFile(t, writer, "new.txt", "x", "more work")
	git(t, writer, "push", "origin", "main")

	s.step()
	if s.note == "" || !strings.Contains(s.note, "uncommitted") {
		t.Errorf("dirty tree sync note = %q, want an honest skip reason", s.note)
	}
	if _, err := os.Stat(filepath.Join(reader, "new.txt")); err == nil {
		t.Error("dirty working tree was fast-forwarded — must never happen")
	}
	if got := git(t, reader, "rev-parse", "origin/main"); got != git(t, writer, "rev-parse", "origin/main") {
		t.Error("refs must stay fresh even when the pull is skipped")
	}

	// On a feature branch: same contract, different reason.
	if err := os.Remove(filepath.Join(reader, "local.txt")); err != nil {
		t.Fatal(err)
	}
	git(t, reader, "checkout", "-b", "feature/tb-t2-local")
	s.step()
	if !strings.Contains(s.note, "feature/tb-t2-local") {
		t.Errorf("feature checkout sync note = %q, want it to name the branch", s.note)
	}
}

func TestSyncHeadersReportFreshnessAndFailure(t *testing.T) {
	_, reader := remoteFixture(t)
	s := &syncer{repo: reader, every: 30 * time.Second}
	s.step()

	rec := httptest.NewRecorder()
	s.headers(rec.Header())
	if rec.Header().Get("X-Truthboard-Sync-Every") != "30" {
		t.Errorf("Sync-Every = %q, want 30", rec.Header().Get("X-Truthboard-Sync-Every"))
	}
	if rec.Header().Get("X-Truthboard-Sync-At") == "" {
		t.Error("Sync-At missing after a successful fetch")
	}

	// A broken remote must surface, not pretend freshness.
	git(t, reader, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone"))
	s.step()
	rec = httptest.NewRecorder()
	s.headers(rec.Header())
	if rec.Header().Get("X-Truthboard-Sync-Err") == "" {
		t.Error("Sync-Err missing after a failing fetch")
	}
}
