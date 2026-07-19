package web

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSyncGroupHeadersNeverLetAStaleSpokeHide: the At header is the oldest
// member fetch, and disappears entirely while any member has never fetched.
func TestSyncGroupHeadersNeverLetAStaleSpokeHide(t *testing.T) {
	hub := &syncer{every: 30 * time.Second}
	api := &syncer{name: "api", proofOnly: true}
	g := &syncGroup{members: []*syncer{hub, api}}

	h := http.Header{}
	hub.at = time.Now()
	g.headers(h)
	if h.Get("X-Truthboard-Sync-At") != "" {
		t.Fatal("At must be omitted while a spoke has never fetched — a fresh hub must not mask it")
	}
	if !strings.Contains(h.Get("X-Truthboard-Sync-Note"), "api: not fetched yet") {
		t.Fatalf("note should name the unfetched spoke, got %q", h.Get("X-Truthboard-Sync-Note"))
	}

	h = http.Header{}
	api.at = time.Now().Add(-10 * time.Minute) // older than the hub
	g.headers(h)
	at, err := time.Parse(time.RFC3339, h.Get("X-Truthboard-Sync-At"))
	if err != nil {
		t.Fatalf("At header unparseable: %v", err)
	}
	if time.Since(at) < 9*time.Minute {
		t.Fatalf("At must be the OLDEST member fetch, got %s", h.Get("X-Truthboard-Sync-At"))
	}

	h = http.Header{}
	api.err = "fetch: connection refused"
	g.headers(h)
	if !strings.Contains(h.Get("X-Truthboard-Sync-Err"), "api: fetch: connection refused") {
		t.Fatalf("spoke errors must carry the spoke name, got %q", h.Get("X-Truthboard-Sync-Err"))
	}
}

// TestSyncerClonesSpokeOnFirstStep: a spoke syncer with a remote and no
// clone yet mirrors it on its first step, then fetches like any other.
func TestSyncerClonesSpokeOnFirstStep(t *testing.T) {
	src := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", src}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "T")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "init")

	clone := filepath.Join(t.TempDir(), "spokes", "api")
	s := &syncer{name: "api", repo: clone, remoteURL: src, proofOnly: true}
	s.step()

	s.mu.Lock()
	at, errMsg := s.at, s.err
	s.mu.Unlock()
	if errMsg != "" {
		t.Fatalf("step failed: %s", errMsg)
	}
	if at.IsZero() {
		t.Fatal("fetch time not recorded")
	}
	if _, err := os.Stat(clone); err != nil {
		t.Fatalf("mirror clone missing: %v", err)
	}
	out, err := exec.Command("git", "-C", clone, "rev-parse", "--verify", "main").CombinedOutput()
	if err != nil {
		t.Fatalf("clone has no main: %v\n%s", err, out)
	}
}
