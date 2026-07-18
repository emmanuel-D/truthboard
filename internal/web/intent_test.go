package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// originAndClone builds the shared-board shape: a bare origin with one
// seed commit, and a server-side clone of it.
func originAndClone(t *testing.T) (origin, clone string) {
	t.Helper()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	git(t, root, "init", "--bare", "-b", "main", origin)

	seed := filepath.Join(root, "seed")
	git(t, root, "init", "-b", "main", seed)
	git(t, seed, "config", "user.email", "t@t.co")
	git(t, seed, "config", "user.name", "T")
	git(t, seed, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", "-A")
	git(t, seed, "commit", "-m", "chore: init")
	git(t, seed, "remote", "add", "origin", origin)
	git(t, seed, "push", "--quiet", "-u", "origin", "main")

	clone = filepath.Join(root, "board-clone")
	git(t, root, "clone", "--quiet", origin, clone)
	git(t, clone, "config", "commit.gpgsign", "false")
	return origin, clone
}

func TestEditTokenGatesWritesAndLandsOnOrigin(t *testing.T) {
	origin, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	// The token never gates reads, and the page learns editing is possible.
	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Truthboard-Readonly") != "" ||
		resp.Header.Get("X-Truthboard-Edit") != "token" {
		t.Errorf("board read = %d readonly=%q edit=%q, want 200 with only X-Truthboard-Edit: token",
			resp.StatusCode, resp.Header.Get("X-Truthboard-Readonly"), resp.Header.Get("X-Truthboard-Edit"))
	}

	// Without the token (or with a wrong one) writes still refuse.
	for _, token := range []string{"", "wrong"} {
		req, _ := http.NewRequest("POST", srv.URL+"/api/specs", strings.NewReader(`{"title":"x"}`))
		if token != "" {
			req.Header.Set("X-Truthboard-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "edit token") {
			t.Errorf("write with token %q = %d %q, want 403 citing the edit token", token, resp.StatusCode, body)
		}
	}

	// With the token, the story is created, committed, and pushed to origin.
	req, _ := http.NewRequest("POST", srv.URL+"/api/specs", strings.NewReader(
		`{"title":"Phone story","owner":"emmanuel","priority":1,"body":"## Goal\n\nFrom the road.\n\n## Acceptance\n\n- [ ] works"}`))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID        string `json:"id"`
		PushError string `json:"push_error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || created.ID == "" || created.PushError != "" {
		t.Fatalf("create = %d %+v", resp.StatusCode, created)
	}

	msg := git(t, origin, "log", "main", "-n", "1", "--format=%B")
	if !strings.Contains(msg, "Phone story") || !strings.Contains(msg, created.ID) {
		t.Errorf("origin tip message %q must name the story and its id", msg)
	}
	// A trailer on the integration branch would derive the story done —
	// writing intent must never fabricate proof.
	if strings.Contains(msg, "Spec: "+created.ID) {
		t.Errorf("origin tip message %q carries the spec trailer", msg)
	}
	files := git(t, origin, "show", "--name-only", "--format=", "main")
	if !strings.Contains(files, ".truthboard/specs/"+created.ID) {
		t.Errorf("origin tip touches %q, want only the spec file", files)
	}

	// Reads of the created spec need no token.
	resp, err = http.Get(srv.URL + "/api/specs/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("tokenless read of created spec = %d, want 200", resp.StatusCode)
	}

	// Concurrent edit safety: origin moves under the board (someone lands
	// work from another clone); the next board edit rebases on top of it.
	other := filepath.Join(t.TempDir(), "other")
	git(t, filepath.Dir(other), "clone", "--quiet", origin, other)
	git(t, other, "config", "user.email", "o@o.co")
	git(t, other, "config", "user.name", "O")
	git(t, other, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(other, "code.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, other, "add", "-A")
	git(t, other, "commit", "-m", "real work")
	git(t, other, "push", "--quiet", "origin", "main")

	req, _ = http.NewRequest("PUT", srv.URL+"/api/specs/"+created.ID, strings.NewReader(`{"priority":2}`))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var updated struct {
		Priority  int    `json:"priority"`
		PushError string `json:"push_error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || updated.Priority != 2 || updated.PushError != "" {
		t.Fatalf("update over moved origin = %d %+v", resp.StatusCode, updated)
	}
	log := git(t, origin, "log", "main", "--format=%s")
	if !strings.Contains(log, "real work") || strings.Index(log, "real work") < strings.Index(log, "edited on the shared board") {
		t.Errorf("origin log %q: the board's edit must sit rebased on top of the concurrent work", log)
	}
}

func TestEditTokenPushFailureSurfaces(t *testing.T) {
	origin, clone := originAndClone(t)
	// Origin disappears: commits still land on the board's clone, but the
	// push error must reach the response, not just a log.
	if err := os.RemoveAll(origin); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/specs", strings.NewReader(`{"title":"Stranded story"}`))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID        string `json:"id"`
		PushError string `json:"push_error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || created.ID == "" {
		t.Fatalf("create with dead origin = %d %+v, want 200 with the spec saved", resp.StatusCode, created)
	}
	if created.PushError == "" {
		t.Error("push against a dead origin must surface in push_error")
	}
	// The intent is committed locally either way — nothing is lost.
	msg := git(t, clone, "log", "-n", "1", "--format=%s")
	if !strings.Contains(msg, "Stranded story") {
		t.Errorf("clone tip %q: the edit must be committed even when the push fails", msg)
	}
}
