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

// del issues an authenticated DELETE and returns status plus body.
func del(t *testing.T, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+path, nil)
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(body))
}

// create posts a story and returns its id.
func create(t *testing.T, srv *httptest.Server, title string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/specs", strings.NewReader(`{"title":`+strconv_Quote(title)+`}`))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ID == "" {
		t.Fatal("create returned no id")
	}
	return out.ID
}

func strconv_Quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestDeleteRetiresAStoryAndLandsOnOrigin covers the case the route exists
// for: a story created by mistake, nothing in git pointing at it. Before
// this, clearing one meant deleting the file from a clone and pushing by
// hand — which is exactly what a probe story on a live board took.
func TestDeleteRetiresAStoryAndLandsOnOrigin(t *testing.T) {
	origin, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	id := create(t, srv, "created by mistake")
	if _, err := os.Stat(filepath.Join(clone, ".truthboard", "specs")); err != nil {
		t.Fatal(err)
	}

	code, body := del(t, srv, "/api/specs/"+id)
	if code != 200 {
		t.Fatalf("delete = %d: %s", code, body)
	}
	if strings.Contains(body, "push_error") {
		t.Errorf("the deletion did not reach origin: %s", body)
	}
	if !strings.Contains(body, "git revert") {
		t.Errorf("the answer must name the undo, got: %s", body)
	}

	// Gone from the board's clone...
	if files := specFiles(t, clone); len(files) != 0 {
		t.Errorf("spec file survived the delete: %v", files)
	}
	// ...and gone on origin, as a commit anyone can revert.
	checkout := filepath.Join(t.TempDir(), "verify")
	git(t, t.TempDir(), "clone", "--quiet", origin, checkout)
	if files := specFiles(t, checkout); len(files) != 0 {
		t.Errorf("deletion never reached origin: %v", files)
	}
	subject := git(t, checkout, "log", "-1", "--format=%s")
	if !strings.Contains(subject, id) || !strings.HasPrefix(subject, "Retire:") {
		t.Errorf("deletion commit subject = %q", subject)
	}
	// A trailer here would derive the story as done on its way out.
	full := git(t, checkout, "log", "-1", "--format=%B")
	if strings.Contains(full, "Spec: "+id) {
		t.Errorf("the deletion commit carries a Spec: trailer, which would derive the deleted story as done:\n%s", full)
	}
}

// TestDeleteRefusedWhileGitStillPointsAtTheStory is the guard that matters:
// branches and trailers are facts, and deleting the promise while the
// evidence survives leaves work on the board nobody can account for.
func TestDeleteRefusedWhileGitStillPointsAtTheStory(t *testing.T) {
	_, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	id := create(t, srv, "work that actually started")

	// Someone starts the work: a branch carrying the id is proof enough.
	git(t, clone, "checkout", "--quiet", "-b", "feature/"+id+"-in-flight")
	commitFile(t, clone, "work.txt", "wip", "feat: start\n\nSpec: "+id)
	git(t, clone, "checkout", "--quiet", "main")

	code, body := del(t, srv, "/api/specs/"+id)
	if code != http.StatusConflict {
		t.Fatalf("delete of a story with proof = %d, want 409: %s", code, body)
	}
	if !strings.Contains(body, "feature/"+id+"-in-flight") {
		t.Errorf("the refusal must name what still references the story, got: %s", body)
	}
	if !strings.Contains(body, "force=1") {
		t.Errorf("the refusal must say how to override it, got: %s", body)
	}
	if files := specFiles(t, clone); len(files) != 1 {
		t.Errorf("a refused delete must leave the spec alone, got %v", files)
	}

	// Deliberate override: the operator really does want it retired.
	code, body = del(t, srv, "/api/specs/"+id+"?force=1")
	if code != 200 {
		t.Fatalf("forced delete = %d: %s", code, body)
	}
	if files := specFiles(t, clone); len(files) != 0 {
		t.Errorf("forced delete left the spec behind: %v", files)
	}
}

// TestDeleteRefusedWithoutTheEditToken keeps deletion on the same footing
// as every other intent write: the promise is editable with the token, and
// the proof is not editable at all.
func TestDeleteRefusedWithoutTheEditToken(t *testing.T) {
	_, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	id := create(t, srv, "not yours to delete")

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/specs/"+id, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("unauthenticated delete = %d, want 403", resp.StatusCode)
	}
	if files := specFiles(t, clone); len(files) != 1 {
		t.Errorf("an unauthenticated delete must change nothing, got %v", files)
	}
}

func specFiles(t *testing.T, repo string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, ".truthboard", "specs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}
