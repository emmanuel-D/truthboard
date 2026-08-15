package mirror

import (
	"fmt"
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// fakeForge stands in for GitHub or GitLab. Every decision mirroring makes
// is testable without a network, an account, or somebody's real tracker —
// which is also why the Client interface exists at all.
type fakeForge struct {
	issues  []Issue
	created []string
	updated []int
	closed  []int
	failOn  string // "create" | "update" | "close"
}

func (f *fakeForge) Repo() string           { return "acme/thing" }
func (f *fakeForge) List() ([]Issue, error) { return f.issues, nil }

func (f *fakeForge) Create(title, body string) (int, error) {
	if f.failOn == "create" && len(f.created) == 1 {
		return 0, fmt.Errorf("rate limited")
	}
	f.created = append(f.created, title)
	return 100 + len(f.created), nil
}

func (f *fakeForge) Update(number int, title, body string) error {
	if f.failOn == "update" {
		return fmt.Errorf("token expired")
	}
	f.updated = append(f.updated, number)
	return nil
}

func (f *fakeForge) Close(number int) error {
	if f.failOn == "close" {
		return fmt.Errorf("forbidden")
	}
	f.closed = append(f.closed, number)
	return nil
}

func board(specs ...spec.Spec) (*audit.Result, []spec.Spec) {
	res := &audit.Result{}
	for _, s := range specs {
		done, total := spec.Progress(s.Body)
		st := audit.Planned
		ev := "no matching branch or commit yet"
		if strings.Contains(s.Body, "LANDED") {
			st, ev = audit.Done, "work landed on origin/main"
		}
		res.Specs = append(res.Specs, audit.SpecStatus{
			ID: s.ID, Title: s.Title, Status: st, Evidence: ev, Epic: s.Epic,
			AcceptanceDone: done, AcceptanceTotal: total,
		})
	}
	return res, specs
}

func story(id, title, body string) spec.Spec {
	return spec.Spec{ID: id, Title: title, File: ".truthboard/specs/" + id + "-x.md", Body: body}
}

// TestPlanCreatesUpdatesAndCloses covers the three things mirroring does,
// and the one it must not do: open a second copy of a story it already
// published.
func TestPlanCreatesUpdatesAndCloses(t *testing.T) {
	res, specs := board(
		story("tb-new", "Never mirrored", "## Goal\nA goal.\n\n## Acceptance\n\n- [ ] one\n"),
		story("tb-old", "Already mirrored and current", "## Goal\nStable.\n"),
		story("tb-mov", "Mirrored but changed since", "## Goal\nRewritten.\n"),
		story("tb-fin", "Mirrored and now landed", "## Goal\nLANDED.\n"),
	)

	// Pre-existing issues: one current, one stale, one open-but-landed, and
	// one a human wrote that mirroring must never touch.
	var current, stale, landed Issue
	for _, ss := range res.Specs {
		s := specByID(specs, ss.ID)
		switch ss.ID {
		case "tb-old":
			current = Issue{Number: 7, Title: Title(ss.ID, ss.Title), State: "open", Body: Body(s, ss)}
		case "tb-mov":
			stale = Issue{Number: 8, Title: Title(ss.ID, ss.Title), State: "open", Body: "an older body"}
		case "tb-fin":
			landed = Issue{Number: 9, Title: Title(ss.ID, ss.Title), State: "open", Body: Body(s, ss)}
		}
	}
	human := Issue{Number: 10, Title: "Someone's own bug report", State: "open", Body: "unrelated"}

	p := Build("acme/thing", specs, res, []Issue{current, stale, landed, human})

	if len(p.Create) != 1 || p.Create[0].SpecID != "tb-new" {
		t.Errorf("create = %+v, want only the unmirrored story", p.Create)
	}
	if len(p.Update) != 1 || p.Update[0].Number != 8 {
		t.Errorf("update = %+v, want issue 8 rewritten", p.Update)
	}
	if len(p.Close) != 1 || p.Close[0].Number != 9 {
		t.Errorf("close = %+v, want issue 9 closed", p.Close)
	}
	if strings.Join(p.Unchanged, ",") != "tb-old" {
		t.Errorf("unchanged = %v, want the current one left alone", p.Unchanged)
	}
	// The human's issue is not ours to rewrite.
	for _, a := range append(append(p.Update, p.Close...), p.Create...) {
		if a.Number == 10 {
			t.Error("mirroring touched an issue nobody mirrored")
		}
	}
}

func specByID(specs []spec.Spec, id string) *spec.Spec {
	for i := range specs {
		if specs[i].ID == id {
			return &specs[i]
		}
	}
	return nil
}

// TestReRunIsAFixedPoint is the property that keeps a mirror from becoming
// a pile of duplicates: publish, then publish again, and the second run has
// nothing to do.
func TestReRunIsAFixedPoint(t *testing.T) {
	res, specs := board(
		story("tb-aaa", "One", "## Goal\nG.\n"),
		story("tb-bbb", "Two", "## Goal\nG.\n"),
	)
	f := &fakeForge{}

	first := Build(f.Repo(), specs, res, nil)
	if len(first.Create) != 2 {
		t.Fatalf("first run = %+v, want both created", first.Create)
	}
	if _, err := Apply(f, first); err != nil {
		t.Fatal(err)
	}
	// The forge now holds what was published.
	for i, ss := range res.Specs {
		f.issues = append(f.issues, Issue{
			Number: 100 + i + 1, Title: Title(ss.ID, ss.Title), State: "open",
			Body: Body(specByID(specs, ss.ID), ss),
		})
	}

	second := Build(f.Repo(), specs, res, f.issues)
	if !second.Empty() {
		t.Errorf("re-running would write again: %+v", second)
	}
	if len(second.Unchanged) != 2 {
		t.Errorf("unchanged = %v, want both recognised as already mirrored", second.Unchanged)
	}
	// And the mapping survives a fresh clone, because there is no mapping:
	// the id is in the issue title, so a machine that has never run this
	// before reaches the same conclusion.
	if !second.Empty() {
		t.Error("recognition must not depend on local state")
	}
}

// TestBodySaysItIsAMirror: the failure mode of every sync tool ever written
// is somebody editing the copy.
func TestBodySaysItIsAMirror(t *testing.T) {
	res, specs := board(story("tb-doc", "Documented", "## Goal\nWhy.\n\n## Acceptance\n\n- [x] done — proof: `TestX`\n- [ ] not yet\n"))
	body := Body(&specs[0], res.Specs[0])

	for _, want := range []string{
		"Mirror of `.truthboard/specs/tb-doc-x.md`",
		"edit the story there, not here",
		"Status: planned",
		"Derived from git",
		"1 of 2 signed off",
		"- [x] done",
		"- [ ] not yet",
		"proof: `TestX`",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body missing %q:\n%s", want, body)
		}
	}
}

// TestApplyReportsHowFarItGot: a half-mirrored forge is a real outcome, and
// the only unacceptable version of it is the silent one.
func TestApplyReportsHowFarItGot(t *testing.T) {
	res, specs := board(
		story("tb-1", "First", "## Goal\nG.\n"),
		story("tb-2", "Second", "## Goal\nG.\n"),
		story("tb-3", "Third", "## Goal\nG.\n"),
	)
	f := &fakeForge{failOn: "create"}

	done, err := Apply(f, Build(f.Repo(), specs, res, nil))
	if err == nil {
		t.Fatal("a failing forge must be an error")
	}
	if !strings.Contains(err.Error(), "tb-2") {
		t.Errorf("the error must name the story it stopped on: %v", err)
	}
	if len(done.Created) != 1 || !strings.Contains(done.Created[0], "tb-1") {
		t.Errorf("applied = %+v, want the one issue that was written reported", done)
	}
	if !strings.Contains(done.Summary(), "1 created") {
		t.Errorf("summary = %q, want it to say what reached the forge", done.Summary())
	}
}

// TestPlanWritesNothing is the dry-run guarantee, stated as a property:
// building a plan touches no forge at all.
func TestPlanWritesNothing(t *testing.T) {
	res, specs := board(story("tb-dry", "Dry", "## Goal\nG.\n"))
	f := &fakeForge{}

	p := Build(f.Repo(), specs, res, nil)
	if len(p.Create) != 1 {
		t.Fatalf("plan = %+v", p)
	}
	if len(f.created)+len(f.updated)+len(f.closed) != 0 {
		t.Errorf("building a plan wrote to the forge: %+v", f)
	}
}
