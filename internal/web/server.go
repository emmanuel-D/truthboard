// Package web serves the read-only board for the PM/PO audience. Strictly
// read-only by construction: every request that could mutate anything is
// rejected before routing, and there is nothing to mutate anyway — the page
// renders the derived audit result and nothing else.
package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/forge"
)

//go:embed index.html
var indexHTML []byte

// boardCache recomputes the audit at most once per interval so a polling
// browser tab never turns into a git-subprocess storm.
type boardCache struct {
	repo     string
	useForge bool
	interval time.Duration

	mu   sync.Mutex
	body []byte
	at   time.Time
}

func (c *boardCache) get() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.body != nil && time.Since(c.at) < c.interval {
		return c.body, nil
	}
	res, err := audit.Audit(c.repo, audit.Options{})
	if err != nil {
		return nil, err
	}
	if c.useForge {
		if data, ok := forge.Fetch(c.repo); ok {
			audit.EnrichWithForge(res, data, audit.Options{})
		}
	}
	body, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}
	c.body, c.at = body, time.Now()
	return body, nil
}

// Handler returns the read-only HTTP handler; exposed for tests. The repo
// path is resolved to absolute so the page never labels the board "." —
// the audit result carries the path the viewer should recognize.
func Handler(repo string, useForge bool, version string) http.Handler {
	if abs, err := filepath.Abs(repo); err == nil {
		repo = abs
	}
	interval := 2 * time.Second
	if useForge {
		interval = 15 * time.Second // forge APIs are slow and rate-limited
	}
	cache := &boardCache{repo: repo, useForge: useForge, interval: interval}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/api/board", func(w http.ResponseWriter, r *http.Request) {
		body, err := cache.get()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})

	// The read-only guard sits in front of routing: anything but GET/HEAD
	// is rejected, so a write endpoint cannot exist even by accident.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "truthboard is read-only: statuses are derived from git, never typed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Truthboard-Version", version)
		mux.ServeHTTP(w, r)
	})
}

// Serve listens on localhost only and optionally opens the browser.
func Serve(repo string, port int, useForge, openBrowser bool, version string) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	url := "http://" + addr
	fmt.Printf("truthboard ui — read-only board for %s\n%s (ctrl-c to stop)\n", repo, url)
	if openBrowser {
		browse(url)
	}
	return http.Serve(ln, Handler(repo, useForge, version))
}

func browse(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start() // best effort; the URL is printed either way
}
