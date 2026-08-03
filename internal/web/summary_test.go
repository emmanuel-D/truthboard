package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/report"
)

// The panel and the pasted document must never come to disagree about what
// happened. They cannot, because both are rendered from one Summary the
// audit built — this asserts the board really does serve that object and
// not a second computation of its own.
func TestBoardPanelAndCLISummaryAgree(t *testing.T) {
	_, clone := originAndClone(t)
	writeSummarySpecs(t, clone)

	srv := httptest.NewServer(Handler(clone, Options{Version: "test"}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	var served audit.Result
	if err := json.Unmarshal(body, &served); err != nil {
		t.Fatal(err)
	}
	if served.Summary == nil {
		t.Fatal("the board serves no summary; the panel would have nothing to render")
	}

	// The same repo, summarised the way the CLI does it.
	direct, err := audit.Audit(clone, audit.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(direct.Summary)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(served.Summary)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("the board's summary differs from the one the CLI renders:\n board: %s\n   cli: %s", got, want)
	}

	// And every story the panel would list appears in the markdown, so a
	// reader of one is never told less than a reader of the other.
	var md strings.Builder
	if err := report.Summary(&md, served.Summary, false); err != nil {
		t.Fatal(err)
	}
	for _, group := range [][]audit.SummaryItem{
		served.Summary.Delivered, served.Summary.InFlight,
		served.Summary.Paused, served.Summary.NotStarted, served.Summary.Broken,
	} {
		for _, it := range group {
			if !strings.Contains(md.String(), it.Title) {
				t.Errorf("story %q is on the panel but not in the document:\n%s", it.Title, md.String())
			}
			if it.Reason != "" && !strings.Contains(md.String(), it.Reason) {
				t.Errorf("reason %q is on the panel but not in the document", it.Reason)
			}
		}
	}
}

func writeSummarySpecs(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, ".truthboard", "specs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"tb-w001.md": "---\nid: tb-w001\ntitle: Weekly invoice email\npoints: 5\n---\n\n## Goal\nx\n",
		"tb-w002.md": "---\nid: tb-w002\ntitle: Refund flow\npoints: 3\nhold: waiting on the payments provider\n---\n\n## Goal\nx\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-m", "chore: stories")
}
