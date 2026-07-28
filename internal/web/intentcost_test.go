package web

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Saving a story is slow because it talks to the forge, so the honest unit
// of cost is round-trips to the remote, not wall time on a local fixture.
// git reaches a remote by running upload-pack (fetch) or receive-pack
// (push) on the far side, and remote.origin.uploadpack/receivepack say
// which binary that is — so a counting shim there counts every round-trip
// exactly, with no timing noise.
type remoteCost struct{ dir string }

func countingRemote(t *testing.T, repo string) *remoteCost {
	t.Helper()
	c := &remoteCost{dir: t.TempDir()}
	for _, pack := range []string{"upload-pack", "receive-pack"} {
		script := filepath.Join(c.dir, pack)
		body := fmt.Sprintf("#!/bin/sh\nprintf '%s\\n' >> %q\nexec git-%s \"$@\"\n",
			pack, filepath.Join(c.dir, "count"), pack)
		if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		git(t, repo, "config", "remote.origin."+strings.ReplaceAll(pack, "-", ""), script)
	}
	return c
}

// trips returns how many times the remote has been contacted, by kind.
func (c *remoteCost) trips(t *testing.T) (fetches, pushes int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(c.dir, "count"))
	if os.IsNotExist(err) {
		return 0, 0
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(raw)) {
		switch line {
		case "upload-pack":
			fetches++
		case "receive-pack":
			pushes++
		}
	}
	return fetches, pushes
}

// TestSaveCostsOneRoundTripWhenNobodyElsePushed pins the common case: the
// board is the only writer most of the time, and the sync loop has already
// fetched within the last interval, so a save that unconditionally rebases
// first pays a second round-trip for a rebase that does nothing.
func TestSaveCostsOneRoundTripWhenNobodyElsePushed(t *testing.T) {
	_, reader := remoteFixture(t)
	cost := countingRemote(t, reader)
	c := &committer{repo: reader}

	writeSpec(t, reader, "tb-aaa1")
	start := time.Now()
	if err := c.land(filepath.Join(".truthboard", "specs", "tb-aaa1.md"), "Intent: one"); err != nil {
		t.Fatalf("land: %v", err)
	}
	elapsed := time.Since(start)

	fetches, pushes := cost.trips(t)
	t.Logf("one save, uncontended: %d fetch + %d push round-trips in %s", fetches, pushes, elapsed)
	if pushes != 1 {
		t.Errorf("a save must push exactly once, got %d", pushes)
	}
	if fetches != 0 {
		t.Errorf("nobody else pushed, so no fetch is needed; got %d — each one is a forge round-trip a phone waits out", fetches)
	}
}

// TestSaveRebasesOnlyWhenTheRemoteMoved is the other half: skipping the
// rebase must never cost correctness. When someone else really has landed
// intent, the save has to notice, rebase, and still arrive.
func TestSaveRebasesOnlyWhenTheRemoteMoved(t *testing.T) {
	writer, reader := remoteFixture(t)
	cost := countingRemote(t, reader)
	c := &committer{repo: reader}

	// Another clone lands intent first, so the board's push is stale.
	commitFile(t, writer, "other.md", "theirs", "someone else's story")
	git(t, writer, "push", "origin", "main")

	writeSpec(t, reader, "tb-aaa2")
	if err := c.land(filepath.Join(".truthboard", "specs", "tb-aaa2.md"), "Intent: two"); err != nil {
		t.Fatalf("land against a moved remote: %v", err)
	}

	fetches, pushes := cost.trips(t)
	t.Logf("one save, remote moved: %d fetch + %d push round-trips", fetches, pushes)
	if fetches == 0 {
		t.Error("a rejected push must be followed by a rebase, which fetches")
	}

	// Both stories must exist on origin: ours, and the one we rebased onto.
	git(t, writer, "pull", "--quiet", "origin", "main")
	for _, want := range []string{"other.md", filepath.Join(".truthboard", "specs", "tb-aaa2.md")} {
		if _, err := os.Stat(filepath.Join(writer, want)); err != nil {
			t.Errorf("%s did not survive the rebase: %v", want, err)
		}
	}
}

// TestConcurrentSavesEachArrive keeps the queueing honest: git on one
// working tree is serial and always will be, so the guarantee is that
// every save lands, not that they overlap.
func TestConcurrentSavesEachArrive(t *testing.T) {
	writer, reader := remoteFixture(t)
	cost := countingRemote(t, reader)
	c := &committer{repo: reader}

	const n = 4
	errs := make(chan error, n)
	start := time.Now()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("tb-cc%02d", i)
		go func() {
			writeSpec(t, reader, id)
			errs <- c.land(filepath.Join(".truthboard", "specs", id+".md"), "Intent: "+id)
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent save: %v", err)
		}
	}
	elapsed := time.Since(start)

	fetches, pushes := cost.trips(t)
	t.Logf("%d concurrent saves: %d fetch + %d push round-trips in %s", n, fetches, pushes, elapsed)

	git(t, writer, "pull", "--quiet", "origin", "main")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("tb-cc%02d", i)
		if _, err := os.Stat(filepath.Join(writer, ".truthboard", "specs", id+".md")); err != nil {
			t.Errorf("%s never reached origin: %v", id, err)
		}
	}
}

func writeSpec(t *testing.T, repo, id string) {
	t.Helper()
	dir := filepath.Join(repo, ".truthboard", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: " + id + "\ntitle: " + id + "\n---\n\n## Goal\n\nmeasured\n"
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestIntentWriteReachesSSEClients closes the gap that made a new story
// look like a sync failure: the broadcaster fired on webhook pushes only,
// so a story written on one phone sat invisible on every other viewer's
// board until that viewer's own poll came round.
func TestIntentWriteReachesSSEClients(t *testing.T) {
	_, clone := originAndClone(t)
	board := Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"})
	t.Cleanup(board.Wait)
	srv := httptest.NewServer(board)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	awaitLine := func(want string) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			select {
			case l := <-lines:
				if l == want {
					return
				}
			case <-deadline:
				t.Fatalf("SSE stream never carried %q — other viewers wait out their own poll", want)
			}
		}
	}
	awaitLine(": connected")

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/specs",
		strings.NewReader(`{"title":"a story someone else should see at once"}`))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	req.Header.Set("Content-Type", "application/json")
	wresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, wresp.Body)
	wresp.Body.Close()
	if wresp.StatusCode != 200 {
		t.Fatalf("intent write = %d", wresp.StatusCode)
	}

	awaitLine("data: refresh")
}
