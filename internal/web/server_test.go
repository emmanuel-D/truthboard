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
	srv := httptest.NewServer(Handler(fixtureRepo(t), false, "test"))
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
	if !strings.Contains(html, "Derived from git, never typed") {
		t.Error("page must carry the derived-never-typed banner")
	}
	if strings.Contains(html, "<script src=") || strings.Contains(html, `rel="stylesheet"`) {
		t.Error("page must be fully self-contained (go:embed, no external assets)")
	}
}

func TestEveryWriteMethodRejected(t *testing.T) {
	srv := httptest.NewServer(Handler(fixtureRepo(t), false, "test"))
	defer srv.Close()

	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		for _, path := range []string{"/", "/api/board", "/api/anything"} {
			req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader("{}"))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405 (read-only by construction)", method, path, resp.StatusCode)
			}
		}
	}
}
