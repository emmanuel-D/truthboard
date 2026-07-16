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
	"strings"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/forge"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
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

// invalidate forces the next get to recompute — called after intent writes
// so the board reflects an edit on the very next poll.
func (c *boardCache) invalidate() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
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

// Handler returns the board handler; exposed for tests. The repo path is
// resolved to absolute so the page never labels the board "." — the audit
// result carries the path the viewer should recognize.
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
		w.Header().Set("X-Truthboard-Dirty", fmt.Sprint(dirtySpecs(repo)))
		w.Write(body)
	})
	mux.HandleFunc("/api/specs", specCreate(repo, cache.invalidate))
	mux.HandleFunc("/api/specs/", specItem(repo, cache.invalidate))

	// The write guard sits in front of routing: the promise (spec intent,
	// under /api/specs) is editable; the proof (everything else) is not.
	// A status has no route by which it could be written.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := r.Method == http.MethodGet || r.Method == http.MethodHead ||
			(r.Method == http.MethodPost && r.URL.Path == "/api/specs") ||
			(r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/specs/"))
		if !allowed {
			http.Error(w, "statuses are derived from git, never typed; only spec intent under /api/specs is writable", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Truthboard-Version", version)
		mux.ServeHTTP(w, r)
	})
}

// dirtySpecs counts uncommitted changes under .truthboard/specs so the page
// can nudge someone to commit intent edits. --no-optional-locks: a board
// polling every few seconds must never take the index lock out from under
// someone's real git commands.
func dirtySpecs(repo string) int {
	out, ok := gitrepo.Try(repo, "--no-optional-locks", "status", "--porcelain", "--", ".truthboard")
	if !ok || out == "" {
		return 0
	}
	return len(strings.Split(out, "\n"))
}

type specPayload struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owner    string   `json:"owner"`
	Branch   string   `json:"branch"`
	Epic     string   `json:"epic"`
	Priority int      `json:"priority"`
	Paths    []string `json:"paths"`
	Body     string   `json:"body"`
}

func payload(s *spec.Spec) specPayload {
	return specPayload{ID: s.ID, Title: s.Title, Owner: s.Owner, Branch: s.Branch,
		Epic: s.Epic, Priority: s.Priority, Paths: s.Paths, Body: s.Body}
}

// decodeIntent rejects unknown fields so a "status" in the payload fails
// loudly — same contract as the MCP server.
func decodeIntent(w http.ResponseWriter, r *http.Request, into any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown field") {
			msg += " — intent fields only; statuses are derived from git and cannot be set"
		}
		http.Error(w, msg, http.StatusBadRequest)
		return false
	}
	return true
}

func specCreate(repo string, invalidate func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title    string   `json:"title"`
			Owner    string   `json:"owner"`
			Epic     string   `json:"epic"`
			Priority int      `json:"priority"`
			Paths    []string `json:"paths"`
			Body     string   `json:"body"`
		}
		if !decodeIntent(w, r, &in) {
			return
		}
		if strings.TrimSpace(in.Title) == "" {
			http.Error(w, "a story needs a title", http.StatusBadRequest)
			return
		}
		s, err := spec.New(repo, in.Title, in.Owner)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if in.Body != "" {
			s.Body = in.Body
		}
		s.Epic, s.Priority, s.Paths = in.Epic, in.Priority, in.Paths
		if err := s.Save(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		invalidate()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload(s))
	}
}

func specItem(repo string, invalidate func()) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/specs/")
		s, err := spec.Find(repo, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(payload(s))
			return
		}
		var in struct {
			Title    *string   `json:"title"`
			Owner    *string   `json:"owner"`
			Branch   *string   `json:"branch"`
			Epic     *string   `json:"epic"`
			Priority *int      `json:"priority"`
			Paths    *[]string `json:"paths"`
			Body     *string   `json:"body"`
		}
		if !decodeIntent(w, r, &in) {
			return
		}
		set := func(dst *string, v *string) {
			if v != nil {
				*dst = *v
			}
		}
		set(&s.Title, in.Title)
		set(&s.Owner, in.Owner)
		set(&s.Branch, in.Branch)
		set(&s.Epic, in.Epic)
		set(&s.Body, in.Body)
		if in.Priority != nil {
			s.Priority = *in.Priority
		}
		if in.Paths != nil {
			s.Paths = *in.Paths
		}
		if strings.TrimSpace(s.Title) == "" {
			http.Error(w, "a story needs a title", http.StatusBadRequest)
			return
		}
		if err := s.Save(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		invalidate()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload(s))
	}
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

// Browse opens the URL in the default browser, best effort.
func Browse(url string) { browse(url) }

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
