package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// notifySink collects webhook posts.
type notifySink struct {
	mu    sync.Mutex
	posts []map[string]string
}

func (s *notifySink) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]string
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			t.Errorf("bad notify payload: %v", err)
		}
		s.mu.Lock()
		s.posts = append(s.posts, m)
		s.mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *notifySink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func writeNotifySpec(t *testing.T, repo, id, title string) {
	t.Helper()
	dir := filepath.Join(repo, ".truthboard", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n---\n\n## Goal\nX\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// stalledFixture builds a repo whose spec derives as stalled: a linked
// branch whose only commit is 10 days old.
func stalledFixture(t *testing.T) string {
	repo := fixtureRepo(t)
	writeNotifySpec(t, repo, "tb-no1", "Watched story")
	old := time.Now().AddDate(0, 0, -10).Format(time.RFC3339)
	for _, args := range [][]string{
		{"checkout", "-b", "feature/tb-no1-work"},
		{"commit", "--allow-empty", "-m", "feat: wip\n\nSpec: tb-no1"},
		{"checkout", "main"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+old, "GIT_COMMITTER_DATE="+old)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return repo
}

func TestNotifyTransitionsOnly(t *testing.T) {
	repo := stalledFixture(t)
	sink := &notifySink{}
	n := &notifier{repo: repo, url: sink.server(t).URL}

	// First check: the story is already stalled — baseline, not news.
	n.check()
	if got := sink.count(); got != 0 {
		t.Fatalf("baseline check posted %d notifications, want 0", got)
	}

	// Pretend the last check saw it in-progress: stalled is now a
	// transition and must post exactly once.
	statePath, err := n.statePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte(`{"tb-no1":"in-progress"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	n.check()
	if got := sink.count(); got != 1 {
		t.Fatalf("transition posted %d notifications, want 1", got)
	}
	p := sink.posts[0]
	if p["spec"] != "tb-no1" || p["status"] != "stalled" || p["was"] != "in-progress" {
		t.Errorf("payload = %v", p)
	}
	if !strings.Contains(p["text"], "tb-no1") || !strings.Contains(p["text"], "stalled") {
		t.Errorf("text = %q, want the id and status for Slack readers", p["text"])
	}

	// Steady state: another check, no repeat.
	n.check()
	if got := sink.count(); got != 1 {
		t.Errorf("steady state posted again (total %d), want still 1", got)
	}
}

func TestNotifyRecoveryIsNewsToo(t *testing.T) {
	repo := stalledFixture(t)
	sink := &notifySink{}
	n := &notifier{repo: repo, url: sink.server(t).URL}
	n.check() // baseline: stalled

	// The story lands: merge the branch, so it derives done.
	for _, args := range [][]string{
		{"merge", "--no-ff", "-m", "Merge branch 'feature/tb-no1-work'", "feature/tb-no1-work"},
		{"branch", "-D", "feature/tb-no1-work"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	n.check()
	if got := sink.count(); got != 1 {
		t.Fatalf("recovery posted %d notifications, want 1", got)
	}
	if p := sink.posts[0]; p["status"] != "done" || p["was"] != "stalled" || !strings.HasPrefix(p["text"], "✓") {
		t.Errorf("recovery payload = %v, want done-from-stalled good news", p)
	}
}

func TestNoNotifyURLMeansNoNotifier(t *testing.T) {
	// Handler without NotifyURL must not create state or post anywhere —
	// the absence of the notifier is the behavior, proven by the state
	// file never appearing.
	repo := stalledFixture(t)
	srv := httptest.NewServer(Handler(repo, Options{Version: "test"}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	gitDir, _ := exec.Command("git", "-C", repo, "rev-parse", "--absolute-git-dir").Output()
	if _, err := os.Stat(filepath.Join(strings.TrimSpace(string(gitDir)), "truthboard", "notify-state.json")); !os.IsNotExist(err) {
		t.Error("notify state exists without --notify; the feature must be fully off")
	}
}
