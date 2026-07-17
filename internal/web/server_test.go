package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "t@t.co"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "chore: init"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestBoardEndpointAndPage(t *testing.T) {
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var board struct {
		Integration string `json:"integration_branch"`
		DigestDays  int    `json:"digest_days"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		t.Fatal(err)
	}
	if board.Integration != "main" || board.DigestDays != 14 {
		t.Errorf("board = %+v, want main integration branch", board)
	}

	page, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	raw, err := io.ReadAll(page.Body)
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(strings.ToLower(html), "derived from git, never typed") {
		t.Error("page must carry the derived-never-typed banner")
	}
	if strings.Contains(html, "<script src=") || strings.Contains(html, `rel="stylesheet"`) {
		t.Error("page must be fully self-contained (go:embed, no external assets)")
	}
}

func TestStatusesHaveNoWritableRoute(t *testing.T) {
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test"}))
	defer srv.Close()

	// Everything except spec-intent writes is rejected before routing.
	reject := []struct{ method, path string }{
		{"POST", "/"}, {"PUT", "/"}, {"PATCH", "/"}, {"DELETE", "/"},
		{"POST", "/api/board"}, {"PUT", "/api/board"}, {"DELETE", "/api/board"},
		{"PATCH", "/api/specs"}, {"DELETE", "/api/specs"},
		{"POST", "/api/specs/tb-x"}, {"PATCH", "/api/specs/tb-x"}, {"DELETE", "/api/specs/tb-x"},
		{"PUT", "/api/specs"}, // PUT needs an id
	}
	for _, tc := range reject {
		req, _ := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader("{}"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestSharedBoardIsReadOnly(t *testing.T) {
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Host: "0.0.0.0", Version: "test"}))
	defer srv.Close()

	// Reading works and announces read-only so the page hides editing.
	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Truthboard-Readonly") != "1" {
		t.Errorf("board read = %d readonly=%q, want 200 with X-Truthboard-Readonly: 1",
			resp.StatusCode, resp.Header.Get("X-Truthboard-Readonly"))
	}

	// Intent writes refuse with the reason, before routing.
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/specs"}, {"PUT", "/api/specs/tb-x"},
	} {
		req, _ := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(`{"title":"x"}`))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "read-only") {
			t.Errorf("%s %s = %d %q, want 403 citing read-only", tc.method, tc.path, resp.StatusCode, body)
		}
	}
}

func TestIntentEditingLifecycle(t *testing.T) {
	repo := fixtureRepo(t)
	srv := httptest.NewServer(Handler(repo, Options{Version: "test"}))
	defer srv.Close()

	// PO creates a story in the browser.
	resp, err := http.Post(srv.URL+"/api/specs", "application/json", strings.NewReader(
		`{"title":"Onboarding flow","owner":"po","epic":"activation","priority":1,"body":"## Goal\n\nSmooth signup.\n\n## Acceptance\n\n- [ ] under 60s"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || created.ID == "" {
		t.Fatalf("create = %d %+v", resp.StatusCode, created)
	}

	// PO refines the story.
	req, _ := http.NewRequest("PUT", srv.URL+"/api/specs/"+created.ID,
		strings.NewReader(`{"priority":2,"title":"Onboarding flow v2"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("update = %d", resp.StatusCode)
	}

	// The edit is a plain file change, visible to git.
	matches, _ := filepath.Glob(filepath.Join(repo, ".truthboard", "specs", created.ID+"-*.md"))
	if len(matches) != 1 {
		t.Fatalf("spec file: %v", matches)
	}
	raw, _ := os.ReadFile(matches[0])
	for _, want := range []string{"Onboarding flow v2", "priority: 2", "under 60s"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("spec file missing %q", want)
		}
	}

	// The board reports the uncommitted intent change.
	resp, err = http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if d := resp.Header.Get("X-Truthboard-Dirty"); d == "" || d == "0" {
		t.Errorf("X-Truthboard-Dirty = %q, want > 0 after uncommitted edits", d)
	}

	// Writing a status has no route to succeed — strict decode rejects it.
	req, _ = http.NewRequest("PUT", srv.URL+"/api/specs/"+created.ID,
		strings.NewReader(`{"status":"done"}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "derived") {
		t.Errorf("status write = %d %q, want 400 citing derived statuses", resp.StatusCode, body)
	}
}
