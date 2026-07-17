// Notifications: the board derives that work stalled or regressed, but
// only tells people who open it. A board configured with --notify posts
// status *transitions* to a webhook URL (generic JSON, Slack-compatible)
// so the news travels. Strictly derived — a notification only ever
// repeats what the audit concluded, evidence included.
package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

var notifyClient = &http.Client{Timeout: 10 * time.Second}

// notifier remembers each spec's last derived status and posts changes
// worth interrupting someone for. State lives under .git/truthboard/ —
// per-clone and never committed, like the board lifecycle files.
type notifier struct {
	repo string
	url  string

	mu sync.Mutex // serializes checks; state file is read-modify-write
}

// alertStates are the statuses whose arrival — or departure — is news.
// planned→in-progress→done is the happy path and stays quiet.
var alertStates = map[audit.Status]bool{audit.Stalled: true, audit.Regressed: true}

type transition struct {
	ID       string
	Title    string
	From     audit.Status
	To       audit.Status
	Evidence string
}

// check re-derives the board and posts every transition into or out of an
// alert state since the last check. The first check of a clone records a
// baseline and posts nothing: a story that was already stalled at first
// sight is the starting point, not news.
func (n *notifier) check() {
	n.mu.Lock()
	defer n.mu.Unlock()

	res, err := audit.Audit(n.repo, audit.Options{})
	if err != nil {
		log.Printf("notify: audit failed: %v", err)
		return
	}
	statePath, err := n.statePath()
	if err != nil {
		log.Printf("notify: %v", err)
		return
	}
	seen, hadBaseline := loadState(statePath)

	current := map[string]audit.Status{}
	var news []transition
	for _, s := range res.Specs {
		current[s.ID] = s.Status
		prev, known := seen[s.ID]
		if prev == s.Status {
			continue
		}
		// News: entering an alert state, or a known alerted story leaving
		// one (recovery). Unknown specs on a baselined clone count from
		// planned-like silence — only their alert arrivals matter.
		if alertStates[s.Status] || (known && alertStates[prev]) {
			news = append(news, transition{ID: s.ID, Title: s.Title, From: prev, To: s.Status, Evidence: s.Evidence})
		}
	}
	if err := saveState(statePath, current); err != nil {
		log.Printf("notify: %v", err)
		return
	}
	if !hadBaseline {
		return
	}
	for _, tr := range news {
		n.post(tr)
	}
}

func (n *notifier) run(every time.Duration) {
	for {
		n.check()
		time.Sleep(every)
	}
}

func (n *notifier) post(tr transition) {
	icon := "⚠"
	if !alertStates[tr.To] {
		icon = "✓" // recovery — good news is news too
	}
	from := string(tr.From)
	if from == "" {
		from = "untracked"
	}
	payload, _ := json.Marshal(map[string]string{
		// "text" makes the payload a valid Slack incoming-webhook message;
		// the structured fields ride along for anything that parses JSON.
		"text":     fmt.Sprintf("%s %s %q — %s (was %s): %s", icon, tr.ID, tr.Title, tr.To, from, tr.Evidence),
		"spec":     tr.ID,
		"title":    tr.Title,
		"status":   string(tr.To),
		"was":      from,
		"evidence": tr.Evidence,
	})
	resp, err := notifyClient.Post(n.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("notify: post %s: %v", tr.ID, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("notify: post %s: HTTP %d", tr.ID, resp.StatusCode)
	}
}

func (n *notifier) statePath() (string, error) {
	gitDir, ok := gitrepo.Try(n.repo, "rev-parse", "--absolute-git-dir")
	if !ok {
		return "", fmt.Errorf("cannot resolve git dir for %s", n.repo)
	}
	dir := filepath.Join(gitDir, "truthboard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "notify-state.json"), nil
}

func loadState(path string) (map[string]audit.Status, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]audit.Status{}, false
	}
	var m map[string]audit.Status
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return map[string]audit.Status{}, false
	}
	return m, true
}

func saveState(path string, m map[string]audit.Status) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
