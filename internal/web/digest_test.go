package web

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// digestSink collects digest posts, which carry a text line and the
// structured difference beneath it.
type digestSink struct {
	mu    sync.Mutex
	posts []struct {
		Text string          `json:"text"`
		Diff json.RawMessage `json:"diff"`
	}
}

func (s *digestSink) server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p struct {
			Text string          `json:"text"`
			Diff json.RawMessage `json:"diff"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("bad digest payload: %v", err)
		}
		s.mu.Lock()
		s.posts = append(s.posts, p)
		s.mu.Unlock()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (s *digestSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.posts)
}

func (s *digestSink) last(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.posts) == 0 {
		t.Fatal("no digest was posted")
	}
	return s.posts[len(s.posts)-1].Text
}

// landStory commits a spec file and an implementation file carrying the
// trailer, so the story derives as landed.
func landStory(t *testing.T, repo, id, title string, ticks ...string) {
	t.Helper()
	dir := filepath.Join(repo, ".truthboard", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: " + id + "\ntitle: " + title + "\n---\n\n## Goal\nX\n\n## Acceptance\n\n"
	for i, mark := range ticks {
		body += "- [" + mark + "] criterion " + string(rune('A'+i)) + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, id+".go"), []byte("package p // "+id), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "Deliver " + id + "\n\nSpec: " + id}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// TestDigestBaselinesThenReportsWhatChanged covers the schedule's contract:
// the first run says nothing (the history was already there to read), and
// the next one reports only what happened after it.
func TestDigestBaselinesThenReportsWhatChanged(t *testing.T) {
	repo := fixtureRepo(t)
	sink := &digestSink{}
	d := &digester{repo: repo, url: sink.server(t).URL}

	d.check()
	if got := sink.count(); got != 0 {
		t.Fatalf("the first run posted %d digests, want 0 — it is a baseline", got)
	}

	landStory(t, repo, "tb-dg1", "A story that landed", "x")
	d.check()
	if got := sink.count(); got != 1 {
		t.Fatalf("posted %d digests, want 1", got)
	}
	text := sink.last(t)
	for _, want := range []string{"tb-dg1", "A story that landed", "Landed"} {
		if !strings.Contains(text, want) {
			t.Errorf("digest must name what changed (%q missing):\n%s", want, text)
		}
	}
}

// TestDigestDoesNotRepeatItself is the difference between a digest people
// read and one they mute.
func TestDigestDoesNotRepeatItself(t *testing.T) {
	repo := fixtureRepo(t)
	sink := &digestSink{}
	d := &digester{repo: repo, url: sink.server(t).URL}

	d.check() // baseline
	landStory(t, repo, "tb-dg2", "Landed once", "x")
	d.check()
	d.check()
	d.check()
	if got := sink.count(); got != 1 {
		t.Errorf("the same news was posted %d times, want once", got)
	}
}

// TestDigestStaysSilentWhenNothingChanged: a digest that said "nothing to
// report" every morning would be trained away within a week.
func TestDigestStaysSilentWhenNothingChanged(t *testing.T) {
	repo := fixtureRepo(t)
	sink := &digestSink{}
	d := &digester{repo: repo, url: sink.server(t).URL}
	d.check() // baseline

	// A commit that touches no story at all.
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("churn"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-m", "chore: churn"}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	d.check()
	if got := sink.count(); got != 0 {
		t.Errorf("posted %d digests for a window where nothing happened, want 0", got)
	}
}

// TestDigestNamesUnreadAcceptance is the one item in the digest that asks
// someone to do something.
func TestDigestNamesUnreadAcceptance(t *testing.T) {
	repo := fixtureRepo(t)
	sink := &digestSink{}
	d := &digester{repo: repo, url: sink.server(t).URL}
	d.check() // baseline

	landStory(t, repo, "tb-dg3", "Landed with promises unread", " ", " ", "x")
	d.check()
	text := sink.last(t)
	if !strings.Contains(text, "Landed with acceptance unread") {
		t.Errorf("digest must call out landed work nobody verified:\n%s", text)
	}
	if !strings.Contains(text, "1 of 3") {
		t.Errorf("digest must name the counts:\n%s", text)
	}
}

// TestDigestKeepsUnsentNewsForNextTime: a webhook that was down must not
// cost the news it failed to deliver.
func TestDigestKeepsUnsentNewsForNextTime(t *testing.T) {
	repo := fixtureRepo(t)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer failing.Close()

	d := &digester{repo: repo, url: failing.URL}
	d.check() // baseline
	landStory(t, repo, "tb-dg4", "Landed while the hook was down", "x")
	d.check() // fails to post

	sink := &digestSink{}
	d.url = sink.server(t).URL
	d.check()
	if got := sink.count(); got != 1 {
		t.Fatalf("posted %d digests after recovery, want the missed news delivered once", got)
	}
	if !strings.Contains(sink.last(t), "tb-dg4") {
		t.Errorf("the missed story was dropped:\n%s", sink.last(t))
	}
}

// TestWebhookURLNeverReachesTheLogs holds the line tb-5266 drew, for the
// one URL that is entirely a secret. A Slack webhook carries no userinfo to
// strip — the path *is* the credential — so the whole URL must stay out.
func TestWebhookURLNeverReachesTheLogs(t *testing.T) {
	repo := fixtureRepo(t)
	// Shaped like a real Slack webhook, pointed at a port nothing listens
	// on. A test must never send anything to somebody else's service, and
	// the connection is refused locally without touching the network.
	secret := "http://127.0.0.1:1/services/T0000/B0000/sUp3rS3cr3tT0k3n"

	var logs strings.Builder
	log.SetOutput(&logs)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	d := &digester{repo: repo, url: secret}
	d.check() // baseline
	landStory(t, repo, "tb-dg5", "Landed with a bad hook", "x")
	d.check() // post fails: the host does not resolve or refuses

	out := logs.String()
	if out == "" {
		t.Fatal("expected the failed post to be logged at all")
	}
	if strings.Contains(out, "sUp3rS3cr3tT0k3n") {
		t.Errorf("the webhook secret reached the log:\n%s", out)
	}
	if strings.Contains(out, secret) {
		t.Errorf("the webhook URL reached the log:\n%s", out)
	}
	if !strings.Contains(out, "<webhook>") {
		t.Errorf("the log should say a webhook failed, without saying which:\n%s", out)
	}

	// The same rule for a URL that does carry userinfo.
	if got := safeError(fmt.Errorf(`Post "https://user:tok3n@example.com/hook": dial failed`), ""); strings.Contains(got, "tok3n") {
		t.Errorf("embedded credentials survived redaction: %s", got)
	}
}
