package web

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebhookRequiresTheSecret(t *testing.T) {
	board := Handler(fixtureRepo(t), Options{Version: "test", WebhookSecret: "s3cret"})
	// t.Cleanup is LIFO, so this runs before the fixture TempDir is
	// removed: the refresh finishes writing before its repo vanishes.
	t.Cleanup(board.Wait)
	srv := httptest.NewServer(board)
	defer srv.Close()

	// No secret at all.
	resp, err := http.Post(srv.URL+"/webhook", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("no secret: status %d, want 403", resp.StatusCode)
	}

	// Wrong plain token.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Gitlab-Token", "wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong token: status %d, want 403", resp.StatusCode)
	}

	// Right plain token (GitLab shape).
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Gitlab-Token", "s3cret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("right token: status %d, want 202", resp.StatusCode)
	}

	// GitHub HMAC shape over the exact body.
	body := `{"ref":"refs/heads/main"}`
	mac := hmac.New(sha256.New, []byte("s3cret"))
	mac.Write([]byte(body))
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/webhook", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("valid HMAC: status %d, want 202", resp.StatusCode)
	}
}

func TestWebhookAbsentWithoutSecret(t *testing.T) {
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test"}))
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/webhook", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted {
		t.Error("an unarmed board must not accept webhooks")
	}
}

func TestWebhookWorksOnSharedReadOnlyHost(t *testing.T) {
	board := Handler(fixtureRepo(t), Options{Version: "test", Host: "0.0.0.0", WebhookSecret: "s3cret"})
	// t.Cleanup is LIFO, so this runs before the fixture TempDir is
	// removed: the refresh finishes writing before its repo vanishes.
	t.Cleanup(board.Wait)
	srv := httptest.NewServer(board)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhook?token=s3cret", strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("shared host webhook: status %d, want 202 — the read-only guard must not eat the fetch trigger", resp.StatusCode)
	}
	// Spec writes stay forbidden on the shared host regardless.
	resp, err = http.Post(srv.URL+"/api/specs", "application/json", strings.NewReader(`{"title":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("shared host spec write: status %d, want 403", resp.StatusCode)
	}
}

func TestWebhookPushReachesSSEClients(t *testing.T) {
	board := Handler(fixtureRepo(t), Options{Version: "test", WebhookSecret: "s3cret"})
	// t.Cleanup is LIFO, so this runs before the fixture TempDir is
	// removed: the refresh finishes writing before its repo vanishes.
	t.Cleanup(board.Wait)
	srv := httptest.NewServer(board)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("events content-type = %q", ct)
	}

	lines := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	awaitLine := func(want string) {
		deadline := time.After(10 * time.Second)
		for {
			select {
			case l := <-lines:
				if l == want {
					return
				}
			case <-deadline:
				t.Fatalf("SSE stream never carried %q", want)
			}
		}
	}
	awaitLine(": connected")

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Gitlab-Token", "s3cret")
	wresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, wresp.Body)
	wresp.Body.Close()

	awaitLine("data: refresh")
}

// The flake this guards against appeared once on a loaded CI runner and would
// not reproduce locally, so rather than chase it, assert the guarantee that
// makes it impossible: once Wait returns, no webhook-triggered work is still
// touching the repository. Under -race this also proves the happens-before,
// which is what stops a cleanup racing a refresh mid-write.
func TestWaitAwaitsWebhookRefresh(t *testing.T) {
	board := &Board{}
	started := make(chan struct{})
	release := make(chan struct{})
	done := false

	h := webhook("s3cret", board.spawn, func() {
		close(started)
		<-release
		done = true // read by the main goroutine only after Wait
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("{}"))
	req.Header.Set("X-Gitlab-Token", "s3cret")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202", rec.Code)
	}
	// The response must not claim the refresh is finished — it plainly is not.
	if body := rec.Body.String(); !strings.Contains(body, "background") {
		t.Errorf("response should say the refresh is asynchronous, got %q", body)
	}

	<-started // the refresh is now in flight and the handler has already returned
	if done {
		t.Fatal("the refresh finished synchronously; this test proves nothing")
	}
	close(release)

	board.Wait()
	if !done {
		t.Error("Wait returned while the refresh was still running")
	}
}

// A board with no webhook work must not block its caller.
func TestWaitReturnsImmediatelyWithNoWork(t *testing.T) {
	board := &Board{}
	returned := make(chan struct{})
	go func() { board.Wait(); close(returned) }()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait blocked with no background work")
	}
}
