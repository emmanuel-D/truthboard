//go:build !windows

package lifecycle

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/web"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestStateRoundTripInsideGitDir(t *testing.T) {
	repo := gitRepo(t)
	s := &State{PID: 12345, Port: 1337, URL: "http://127.0.0.1:1337", Started: time.Now()}
	if err := save(repo, s); err != nil {
		t.Fatal(err)
	}
	path, _ := statePath(repo)
	if !strings.Contains(path, filepath.Join(".git", "truthboard")) {
		t.Errorf("state lives at %s, want inside the git dir (never committable)", path)
	}
	loaded, err := Load(repo)
	if err != nil || loaded == nil || loaded.PID != 12345 || loaded.Port != 1337 {
		t.Fatalf("load = %+v, %v", loaded, err)
	}
	if err := Remove(repo); err != nil {
		t.Fatal(err)
	}
	if again, _ := Load(repo); again != nil {
		t.Error("state should be gone after Remove")
	}
	if err := Remove(repo); err != nil {
		t.Errorf("second Remove must be a no-op, got %v", err)
	}
}

func TestStatusCleansStaleState(t *testing.T) {
	repo := gitRepo(t)
	// A PID that is certainly gone: spawn and reap a short-lived process.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	dead := cmd.Process.Pid
	if err := save(repo, &State{PID: dead, Port: 1337, URL: "http://127.0.0.1:1337", Started: time.Now()}); err != nil {
		t.Fatal(err)
	}

	msg, err := Status(repo, "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "stale") {
		t.Errorf("status = %q, want stale-state cleanup", msg)
	}
	if s, _ := Load(repo); s != nil {
		t.Error("stale state should have been removed")
	}
}

func TestStatusAndStopWithNothingRunning(t *testing.T) {
	repo := gitRepo(t)
	if msg, err := Status(repo, "v1.0.0"); err != nil || !strings.Contains(msg, "no detached board") {
		t.Errorf("status = %q, %v", msg, err)
	}
	if msg, err := Stop(repo); err != nil || !strings.Contains(msg, "nothing to stop") {
		t.Errorf("stop = %q, %v", msg, err)
	}
}

func TestAliveOnRealProcess(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Error("this test process is definitely alive")
	}
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if Alive(cmd.Process.Pid) {
		t.Error("a reaped process should not read as alive")
	}
}

func TestDetachRefusesNonGitDir(t *testing.T) {
	if _, err := Detach(t.TempDir(), web.Options{Port: 1337, Version: "test"}); err == nil ||
		!strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("detach outside git = %v, want a clear error", err)
	}
}

// board stands in for a detached board: a live PID (this test process) plus a
// server that answers with whatever version it was given.
func board(t *testing.T, servedVer string) string {
	t.Helper()
	repo := gitRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if servedVer != "" {
			w.Header().Set("X-Truthboard-Version", servedVer)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	if err := save(repo, &State{PID: os.Getpid(), Port: 1337, URL: srv.URL, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestStatusReportsTheServedVersion(t *testing.T) {
	msg, err := Status(board(t, "v0.8.4"), "v0.8.4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "v0.8.4") {
		t.Errorf("status = %q, want the served version named", msg)
	}
	if strings.Contains(msg, "⚠") {
		t.Errorf("a board on the current build must not be flagged stale: %q", msg)
	}
}

// The case this story exists for: an install or `brew upgrade` leaves the
// board on the old build, and until now nothing said so.
func TestStatusFlagsAStaleBoard(t *testing.T) {
	msg, err := Status(board(t, "v0.8.3"), "v0.8.4")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"v0.8.3", "v0.8.4", "truthboard stop", "ui --detach"} {
		if !strings.Contains(msg, want) {
			t.Errorf("status missing %q:\n%s", want, msg)
		}
	}
}

// A board that will not answer is worth reporting, never worth hanging or
// failing over — the rest of the line still has to arrive.
func TestStatusSurvivesAnUnreachableBoard(t *testing.T) {
	repo := gitRepo(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening there now
	if err := save(repo, &State{PID: os.Getpid(), Port: 1337, URL: url, Started: time.Now()}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var msg string
	var err error
	go func() { msg, err = Status(repo, "v0.8.4"); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Status hung on an unreachable board")
	}
	if err != nil {
		t.Fatalf("an unreachable board must not fail status: %v", err)
	}
	for _, want := range []string{"running", "unreadable", "pid"} {
		if !strings.Contains(msg, want) {
			t.Errorf("status missing %q:\n%s", want, msg)
		}
	}
}

// Staleness is decided by what the board serves, never by inspecting the
// binary — the executable it started from is routinely gone by then.
func TestStatusFlagsStaleWithoutTouchingTheExecutable(t *testing.T) {
	msg, err := Status(board(t, "v0.1.0"), "v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "⚠") || !strings.Contains(msg, "v0.1.0") {
		t.Errorf("stale board not reported from the version alone:\n%s", msg)
	}
}

// An older board that sets no version header is reported, not mistaken for
// a stale one — we cannot compare what we were not told.
func TestStatusHandlesABoardWithNoVersionHeader(t *testing.T) {
	msg, err := Status(board(t, ""), "v0.8.4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "unreported") {
		t.Errorf("status = %q, want the missing header reported", msg)
	}
	if strings.Contains(msg, "⚠") {
		t.Errorf("an unknown version is not proof of staleness: %q", msg)
	}
}
