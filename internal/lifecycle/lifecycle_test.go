//go:build !windows

package lifecycle

import (
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

	msg, err := Status(repo)
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
	if msg, err := Status(repo); err != nil || !strings.Contains(msg, "no detached board") {
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
