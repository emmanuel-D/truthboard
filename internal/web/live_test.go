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
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test", WebhookSecret: "s3cret"}))
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
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test", Host: "0.0.0.0", WebhookSecret: "s3cret"}))
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
	srv := httptest.NewServer(Handler(fixtureRepo(t), Options{Version: "test", WebhookSecret: "s3cret"}))
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
