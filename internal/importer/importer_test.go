package importer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCSVExportIsRead covers the format Jira and Linear actually produce,
// with the column names each of them actually uses.
func TestCSVExportIsRead(t *testing.T) {
	path := writeTemp(t, "jira.csv",
		"Issue key,Summary,Description,Assignee,Priority,Status,Labels\n"+
			"PROJ-1,Fix the login loop,Users bounce back to the form,ada,High,To Do,auth,\n"+
			"PROJ-2,Rename the button,,,Low,Done,ui\n")

	items, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("read %d items, want 2", len(items))
	}
	if items[0].Key != "PROJ-1" || items[0].Title != "Fix the login loop" {
		t.Errorf("first item = %+v", items[0])
	}
	if items[0].Owner != "ada" || items[0].Priority != 1 || items[0].State != "open" {
		t.Errorf("first item fields = %+v", items[0])
	}
	if items[1].State != "closed" || items[1].Priority != 3 {
		t.Errorf("second item = %+v, want closed and priority 3", items[1])
	}
}

func TestJSONExportIsRead(t *testing.T) {
	path := writeTemp(t, "linear.json",
		`{"issues":[{"identifier":"ENG-7","title":"Ship the thing","description":"why","priority":"urgent","state":"In Progress","labels":"backend, infra"}]}`)
	items, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("read %d items, want 1", len(items))
	}
	it := items[0]
	if it.Key != "ENG-7" || it.Priority != 1 || it.State != "open" {
		t.Errorf("item = %+v", it)
	}
	if strings.Join(it.Labels, ",") != "backend,infra" {
		t.Errorf("labels = %v", it.Labels)
	}
}

// TestPriorityIsNeverGuessed: an invented priority reorders somebody's
// backlog on import, which is exactly the quiet damage this tool exists to
// prevent.
func TestPriorityIsNeverGuessed(t *testing.T) {
	for _, in := range []string{"", "Somewhat important", "P0-ish", "🔥"} {
		if got := priorityOf(in); got != 0 {
			t.Errorf("priorityOf(%q) = %d, want unset", in, got)
		}
	}
	for in, want := range map[string]int{"P1": 1, "urgent": 1, "Medium": 2, "low": 3} {
		if got := priorityOf(in); got != want {
			t.Errorf("priorityOf(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestClosedItemsAreSkippedByDefault, and every skip is accounted for: a
// command somebody runs once must not silently drop half a backlog.
func TestClosedItemsAreSkippedByDefault(t *testing.T) {
	items := []Item{
		{Key: "a#1", Title: "Open one", State: "open"},
		{Key: "a#2", Title: "Closed one", State: "closed"},
		{Key: "a#3", Title: "", State: "open"},
	}
	p := Build("test", items, nil, false)
	if len(p.New) != 1 || p.New[0].Key != "a#1" {
		t.Errorf("new = %+v, want only the open titled item", p.New)
	}
	if len(p.Skipped) != 2 {
		t.Fatalf("skipped = %+v, want both accounted for", p.Skipped)
	}
	if len(p.New)+len(p.Skipped) != len(items) {
		t.Error("every source item must appear in the plan somewhere")
	}
	if withClosed := Build("test", items, nil, true); len(withClosed.New) != 2 {
		t.Errorf("--closed should bring it: %+v", withClosed.New)
	}
}

// TestReImportDoesNotDuplicateOrOverwrite is the property that makes a
// second run safe: a story imported once belongs to whoever edited it
// since.
func TestReImportDoesNotDuplicateOrOverwrite(t *testing.T) {
	repo := t.TempDir()
	items := []Item{{Key: "github#1", Title: "First thing", Body: "why", State: "open", Labels: []string{"Platform Team"}}}

	first := Build("github", items, nil, false)
	written, err := Write(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 {
		t.Fatalf("wrote %v, want one story", written)
	}

	existing, err := spec.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if existing[0].Imported != "github#1" {
		t.Errorf("provenance = %q, want the source key recorded", existing[0].Imported)
	}
	if existing[0].Epic != "platform-team" {
		t.Errorf("epic = %q, want the first label as a slug", existing[0].Epic)
	}

	// Somebody edits the imported story, then the import runs again.
	edited := existing[0]
	edited.Title = "First thing, rewritten by a human"
	if err := edited.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := spec.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	second := Build("github", items, again, false)
	if len(second.New) != 0 {
		t.Errorf("re-import would write again: %+v", second.New)
	}
	if len(second.Skipped) != 1 || second.Skipped[0].Spec != existing[0].ID {
		t.Errorf("skipped = %+v, want it pointing at the story that already covers it", second.Skipped)
	}
	after, err := spec.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Title != "First thing, rewritten by a human" {
		t.Errorf("the human's edit was overwritten: %q", after[0].Title)
	}
}

// TestImportedStoriesArriveVisiblyIncomplete: most items have no acceptance
// criteria, and a placeholder criterion would read as a real promise.
func TestImportedStoriesArriveVisiblyIncomplete(t *testing.T) {
	repo := t.TempDir()
	p := Build("github", []Item{{Key: "github#9", Title: "Something", Body: "the description", State: "open"}}, nil, false)
	if _, err := Write(repo, p); err != nil {
		t.Fatal(err)
	}
	specs, err := spec.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	s := specs[0]

	if done, total := spec.Progress(s.Body); total != 0 || done != 0 {
		t.Errorf("imported story has %d/%d criteria — a placeholder reads as a real promise", done, total)
	}
	if !strings.Contains(s.Body, "Not imported") {
		t.Errorf("the story must say its acceptance is missing:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "the description") {
		t.Errorf("the source text must be carried across unchanged:\n%s", s.Body)
	}
	// No status came across, and there is nowhere for one to go.
	if strings.Contains(strings.ToLower(s.Body), "status:") {
		t.Errorf("a status was imported:\n%s", s.Body)
	}
}

// TestMappingIsStated: every one of these is a judgement someone else might
// have made differently, and a silent mapping is a surprise waiting in
// somebody's backlog.
func TestMappingIsStated(t *testing.T) {
	p := Build("test", nil, nil, false)
	joined := strings.Join(p.Mapping, "\n")
	for _, want := range []string{"assignee → owner", "first label → epic", "priority", "state → not imported"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the mapping notes must mention %q:\n%s", want, joined)
		}
	}
}
