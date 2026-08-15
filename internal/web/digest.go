package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// The digest is the other half of notifications. Transitions interrupt you
// when one story goes wrong; the digest arrives on a schedule and says what
// changed across the whole board — what landed, what was signed off, and
// which landed work is still carrying promises nobody read back.
//
// It reports the same derived difference `truthboard since` prints, so the
// message in a channel and the answer in a terminal cannot disagree. What
// it keeps locally is one line: the commit it last reported up to, so a
// second run does not repeat itself. That is a bookmark, not a snapshot —
// the board at that commit is still recomputed from the commit.
type digester struct {
	repo string
	url  string

	mu sync.Mutex
}

// check reports everything that happened since the last successful post.
// The first run of a clone records where it started and says nothing: a
// board that shouted its entire history the moment someone turned digests
// on would be noise, and the history was already there to read.
func (d *digester) check() {
	d.mu.Lock()
	defer d.mu.Unlock()

	head, ok := gitrepo.Try(d.repo, "rev-parse", "HEAD")
	if !ok {
		log.Print("digest: cannot resolve HEAD")
		return
	}
	head = strings.TrimSpace(head)

	statePath, err := d.statePath()
	if err != nil {
		log.Printf("digest: %v", err)
		return
	}
	last, hadBaseline := loadDigestState(statePath)
	if !hadBaseline {
		if err := saveDigestState(statePath, head); err != nil {
			log.Printf("digest: %v", err)
		}
		return
	}
	if last == head {
		return // nothing new to derive a difference from
	}

	diff, err := audit.SinceDiff(d.repo, last)
	if err != nil {
		// A bookmark can go stale — a force-push, a rebase, a fresh clone.
		// Rebaselining is the recoverable answer; refusing forever is not.
		log.Printf("digest: cannot compare against %.8s (%v) — rebaselining", last, err)
		if err := saveDigestState(statePath, head); err != nil {
			log.Printf("digest: %v", err)
		}
		return
	}
	// Silence is the right answer to a quiet window. A digest that posted
	// "nothing to report" every morning would be trained away within a week.
	if diff.Quiet() {
		return
	}
	if err := d.post(diff); err != nil {
		// Not saved: an unsent digest must be sent next time, not skipped.
		log.Printf("digest: %v", err)
		return
	}
	if err := saveDigestState(statePath, head); err != nil {
		log.Printf("digest: %v", err)
	}
}

func (d *digester) run(every time.Duration) {
	for {
		d.check()
		time.Sleep(every)
	}
}

// post sends the digest. The text field makes it a valid Slack incoming
// webhook message; the structured difference rides along for anything that
// parses JSON.
func (d *digester) post(diff *audit.Diff) error {
	payload, err := json.Marshal(struct {
		Text string      `json:"text"`
		Diff *audit.Diff `json:"diff"`
	}{Text: digestText(diff), Diff: diff})
	if err != nil {
		return err
	}
	resp, err := notifyClient.Post(d.url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("post: %s", safeError(err, d.url))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("post: HTTP %d", resp.StatusCode)
	}
	return nil
}

// digestText writes the message a person reads in a channel: the headline
// first, then the stories behind it, and the unverified acceptance last
// because it is the part that asks someone to do something.
func digestText(diff *audit.Diff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*Truthboard* — %s\n", diff.Headline())

	list := func(label string, changes []audit.Change) {
		if len(changes) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n*%s*\n", label)
		for i, ch := range changes {
			if i == digestCap {
				fmt.Fprintf(&b, "• … and %d more\n", len(changes)-digestCap)
				break
			}
			fmt.Fprintf(&b, "• `%s` %s", ch.ID, ch.Title)
			if ch.Detail != "" {
				fmt.Fprintf(&b, " — %s", ch.Detail)
			}
			b.WriteString("\n")
		}
	}
	list("Landed", diff.Landed)
	list("Came undone", diff.Unlanded)
	list("Signed off", diff.Verified)
	list("Filed", diff.Filed)
	// Named, with counts, because this is the one item in the digest that is
	// a request: landed work whose acceptance nobody has read back.
	list("Landed with acceptance unread", diff.Unverified)
	return b.String()
}

// digestCap bounds each list in a chat message — a wall of a hundred lines
// is not a digest.
const digestCap = 10

func (d *digester) statePath() (string, error) {
	gitDir, ok := gitrepo.Try(d.repo, "rev-parse", "--absolute-git-dir")
	if !ok {
		return "", fmt.Errorf("cannot resolve git dir for %s", d.repo)
	}
	dir := filepath.Join(gitDir, "truthboard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "digest-state.json"), nil
}

func loadDigestState(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var s struct {
		Reported string `json:"reported_through"`
	}
	if json.Unmarshal(raw, &s) != nil || s.Reported == "" {
		return "", false
	}
	return s.Reported, true
}

func saveDigestState(path, sha string) error {
	raw, err := json.Marshal(struct {
		Reported string `json:"reported_through"`
	}{Reported: sha})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
