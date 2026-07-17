// Live mode: a forge push webhook triggers an immediate fetch + re-derive,
// and connected browsers hear about it over SSE instead of waiting out the
// poll interval. Still read-only — the webhook can only make the board
// *fresher*, never change what it says.
package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// broadcaster fans a "board changed" signal out to every open SSE stream.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan struct{}]bool
}

func newBroadcaster() *broadcaster { return &broadcaster{subs: map[chan struct{}]bool{}} }

func (b *broadcaster) subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subs[ch] = true
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan struct{}) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
}

func (b *broadcaster) notify() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- struct{}{}:
		default: // a slow client already has a pending signal; one is enough
		}
	}
}

// events streams "refresh" over SSE. Read-only by nature, so it passes the
// write guard like any GET.
func events(b *broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		ch := b.subscribe()
		defer b.unsubscribe(ch)
		io.WriteString(w, ": connected\n\n")
		fl.Flush()
		keepalive := time.NewTicker(25 * time.Second)
		defer keepalive.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ch:
				io.WriteString(w, "data: refresh\n\n")
				fl.Flush()
			case <-keepalive.C:
				io.WriteString(w, ": keepalive\n\n")
				fl.Flush()
			}
		}
	}
}

// webhook accepts a forge push notification and triggers refresh. Two
// authentication shapes cover the major forges with one shared secret:
// a plain token (GitLab's X-Gitlab-Token, or ?token= for anything that
// can only set a URL) compared in constant time, or GitHub's HMAC-SHA256
// body signature (X-Hub-Signature-256). Anything else is rejected and
// logged — a webhook endpoint on a shared host must fail loudly.
func webhook(secret string, trigger func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "webhooks POST", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "unreadable body", http.StatusBadRequest)
			return
		}
		if !webhookAuthed(r, secret, body) {
			log.Printf("webhook: rejected %s from %s (bad or missing secret)", r.URL.Path, r.RemoteAddr)
			http.Error(w, "bad or missing webhook secret", http.StatusForbidden)
			return
		}
		go trigger()
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, "fetching\n")
	}
}

func webhookAuthed(r *http.Request, secret string, body []byte) bool {
	eq := func(a, b string) bool {
		return len(a) == len(b) && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	}
	if t := r.Header.Get("X-Gitlab-Token"); t != "" {
		return eq(t, secret)
	}
	if sig := r.Header.Get("X-Hub-Signature-256"); sig != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		return eq(sig, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	if t := r.URL.Query().Get("token"); t != "" {
		return eq(t, secret)
	}
	return false
}
