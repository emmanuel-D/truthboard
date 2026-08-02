package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// delBranch issues an authenticated branch deletion and returns status plus
// body. Params are the query the UI would send.
func delBranch(t *testing.T, srv *httptest.Server, params url.Values, token bool) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/branches?"+params.Encode(), nil)
	if token {
		req.Header.Set("X-Truthboard-Token", "s3cret")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(body))
}

// landedBranch creates a branch, pushes it, merges it into main and pushes
// that — the exact shape the board reports as done and never cleans up.
func landedBranch(t *testing.T, clone, name string) {
	t.Helper()
	git(t, clone, "checkout", "--quiet", "-b", name)
	commitFile(t, clone, strings.ReplaceAll(name, "/", "-")+".txt", "work", "feat: work on "+name)
	git(t, clone, "push", "--quiet", "-u", "origin", name)
	git(t, clone, "checkout", "--quiet", "main")
	git(t, clone, "merge", "--quiet", "--no-ff", "-m", "Merge branch '"+name+"'", name)
	git(t, clone, "push", "--quiet", "origin", "main")
}

func branchExists(t *testing.T, repo, ref string) bool {
	t.Helper()
	return strings.TrimSpace(git(t, repo, "for-each-ref", "--format=%(refname)", ref)) != ""
}

// TestRetireMergedBranchDeletesBothRefs is the case the route exists for:
// work that landed, whose branch nobody removed. Both refs go, and origin's
// really is gone on origin — not merely forgotten locally.
func TestRetireMergedBranchDeletesBothRefs(t *testing.T) {
	origin, clone := originAndClone(t)
	landedBranch(t, clone, "feature/tb-1111-landed")
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	// The board says which refs exist before anything is deleted.
	unit := boardUnit(t, srv, "feature/tb-1111-landed")
	if !unit.Local || !unit.Remote || unit.Status != "done" {
		t.Fatalf("board unit = %+v, want a done branch with both refs", unit)
	}

	code, body := delBranch(t, srv, url.Values{
		"name": {"feature/tb-1111-landed"}, "local": {"1"}, "remote": {"1"},
	}, true)
	if code != 200 {
		t.Fatalf("delete = %d: %s", code, body)
	}
	var out struct {
		Deleted []string `json:"deleted"`
		Failed  []string `json:"failed"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Failed) != 0 {
		t.Errorf("deletion reported failures: %v", out.Failed)
	}
	if len(out.Deleted) != 2 {
		t.Errorf("deleted = %v, want both the local ref and origin's", out.Deleted)
	}

	if branchExists(t, clone, "refs/heads/feature/tb-1111-landed") {
		t.Error("the local branch survived the delete")
	}
	if branchExists(t, clone, "refs/remotes/origin/feature/tb-1111-landed") {
		t.Error("the remote-tracking ref survived the delete")
	}
	if branchExists(t, origin, "refs/heads/feature/tb-1111-landed") {
		t.Error("the branch is still on origin — the deletion never left this machine")
	}
	// And the merge itself is untouched: the story's status is derived from
	// that, so cleanup must never move a card.
	if !strings.Contains(git(t, clone, "log", "--oneline", "main"), "work on feature/tb-1111-landed") {
		t.Error("retiring the branch lost the work it had landed")
	}
}

// TestRetireUnmergedBranchIsRefusedThenForced is the guard that matters: a
// branch with commits nowhere else is the one deletion nobody can undo from
// the board.
func TestRetireUnmergedBranchIsRefusedThenForced(t *testing.T) {
	_, clone := originAndClone(t)
	git(t, clone, "checkout", "--quiet", "-b", "feature/tb-2222-in-flight")
	commitFile(t, clone, "wip.txt", "wip", "feat: unfinished")
	git(t, clone, "push", "--quiet", "-u", "origin", "feature/tb-2222-in-flight")
	git(t, clone, "checkout", "--quiet", "main")

	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	params := url.Values{"name": {"feature/tb-2222-in-flight"}, "local": {"1"}, "remote": {"1"}}
	code, body := delBranch(t, srv, params, true)
	if code != http.StatusConflict {
		t.Fatalf("delete of an unmerged branch = %d, want 409: %s", code, body)
	}
	if !strings.Contains(body, "not merged") || !strings.Contains(body, "force=1") {
		t.Errorf("the refusal must say what is at stake and how to override it, got: %s", body)
	}
	if !branchExists(t, clone, "refs/heads/feature/tb-2222-in-flight") {
		t.Error("a refused delete must leave the branch alone")
	}

	params.Set("force", "1")
	if code, body = delBranch(t, srv, params, true); code != 200 {
		t.Fatalf("forced delete = %d: %s", code, body)
	}
	if branchExists(t, clone, "refs/heads/feature/tb-2222-in-flight") {
		t.Error("the forced delete left the branch behind")
	}
}

// TestIntegrationBranchIsNotDeletable: every status on the board is derived
// from the integration branch. No confirmation, no force flag, no route.
func TestIntegrationBranchIsNotDeletable(t *testing.T) {
	_, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	for _, params := range []url.Values{
		{"name": {"main"}, "remote": {"1"}},
		{"name": {"main"}, "local": {"1"}, "force": {"1"}},
	} {
		code, body := delBranch(t, srv, params, true)
		if code != http.StatusBadRequest {
			t.Errorf("delete of the integration branch %v = %d, want 400: %s", params, code, body)
		}
		if !strings.Contains(body, "integration branch") {
			t.Errorf("the refusal must say why, got: %s", body)
		}
	}
	if !branchExists(t, clone, "refs/heads/main") {
		t.Fatal("main was deleted")
	}
}

// TestCheckedOutBranchKeepsItsLocalRef: git will not delete the branch the
// repository is standing on, and a board that pretended otherwise would
// report a cleanup that never happened.
func TestCheckedOutBranchKeepsItsLocalRef(t *testing.T) {
	_, clone := originAndClone(t)
	landedBranch(t, clone, "feature/tb-3333-landed")
	git(t, clone, "checkout", "--quiet", "feature/tb-3333-landed")

	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	code, body := delBranch(t, srv, url.Values{
		"name": {"feature/tb-3333-landed"}, "local": {"1"}, "remote": {"1"},
	}, true)
	if code != http.StatusConflict {
		t.Fatalf("delete of the checked-out branch = %d, want 409: %s", code, body)
	}
	if !strings.Contains(body, "checked out") {
		t.Errorf("the refusal must name the reason, got: %s", body)
	}
	if !branchExists(t, clone, "refs/heads/feature/tb-3333-landed") {
		t.Error("the checked-out branch was deleted anyway")
	}

	// Origin's ref is a different act, and that one is still available.
	if code, body = delBranch(t, srv, url.Values{
		"name": {"feature/tb-3333-landed"}, "remote": {"1"},
	}, true); code != 200 {
		t.Fatalf("deleting only the origin ref = %d: %s", code, body)
	}
	if !branchExists(t, clone, "refs/heads/feature/tb-3333-landed") {
		t.Error("deleting the origin ref took the local branch with it")
	}
}

// TestBranchDeletionNeedsTheEditToken keeps branch cleanup on exactly the
// footing of every other write: gated on a shared board, refused outright
// when the board serves read-only.
func TestBranchDeletionNeedsTheEditToken(t *testing.T) {
	_, clone := originAndClone(t)
	landedBranch(t, clone, "feature/tb-4444-landed")
	params := url.Values{"name": {"feature/tb-4444-landed"}, "local": {"1"}, "remote": {"1"}}

	armed := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer armed.Close()
	if code, body := delBranch(t, armed, params, false); code != http.StatusForbidden {
		t.Errorf("unauthenticated delete = %d, want 403: %s", code, body)
	}

	readonly := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", Version: "test"}))
	defer readonly.Close()
	if code, body := delBranch(t, readonly, params, true); code != http.StatusForbidden {
		t.Errorf("delete on a read-only board = %d, want 403: %s", code, body)
	}
	if !branchExists(t, clone, "refs/heads/feature/tb-4444-landed") {
		t.Error("a refused delete changed the repository")
	}
}

// boardUnit reads one branch back off the board the page renders from.
func boardUnit(t *testing.T, srv *httptest.Server, name string) struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Local  bool   `json:"local"`
	Remote bool   `json:"remote"`
} {
	t.Helper()
	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var board struct {
		Units []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Local  bool   `json:"local"`
			Remote bool   `json:"remote"`
		} `json:"units"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&board); err != nil {
		t.Fatal(err)
	}
	for _, u := range board.Units {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("no unit %q on the board: %+v", name, board.Units)
	return board.Units[0]
}
