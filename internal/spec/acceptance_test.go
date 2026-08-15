package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const body = `## Goal

Ship the thing.

## Acceptance

- [ ] The board renders
- [x] The CLI ticks a box
- [ ] The docs say so

Notes: a paragraph that is not a criterion, and must survive untouched.
`

func newSpec(t *testing.T) *Spec {
	t.Helper()
	s := &Spec{ID: "tb-test", Title: "Test", Body: body,
		File: filepath.Join(t.TempDir(), "tb-test.md")}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCriteriaReadsStateAndOrder(t *testing.T) {
	cs := Criteria(body)
	if len(cs) != 3 {
		t.Fatalf("got %d criteria, want 3", len(cs))
	}
	if cs[0].N != 1 || cs[0].Checked || cs[0].Text != "The board renders" {
		t.Errorf("criterion 1 = %+v", cs[0])
	}
	if !cs[1].Checked {
		t.Errorf("criterion 2 should read as ticked: %+v", cs[1])
	}
	if done, total := Progress(body); done != 1 || total != 3 {
		t.Errorf("Progress = %d/%d, want 1/3", done, total)
	}
}

func TestSetAcceptanceByIndexTouchesOnlyThatLine(t *testing.T) {
	s := newSpec(t)
	changed, err := s.SetAcceptance([]string{"1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0].N != 1 {
		t.Fatalf("changed = %+v, want criterion 1", changed)
	}
	if done, total := Progress(s.Body); done != 2 || total != 3 {
		t.Errorf("Progress = %d/%d, want 2/3", done, total)
	}
	// The tick must be a one-line diff: prose, headings and the criteria
	// nobody named survive byte for byte.
	if !strings.Contains(s.Body, "Notes: a paragraph that is not a criterion, and must survive untouched.") {
		t.Errorf("prose was rewritten:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "- [ ] The docs say so") {
		t.Errorf("an unnamed criterion changed state:\n%s", s.Body)
	}
	raw, err := os.ReadFile(s.File)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "- [x] The board renders") {
		t.Errorf("the tick did not reach the file:\n%s", raw)
	}
}

func TestSetAcceptanceBySubstringAndAll(t *testing.T) {
	s := newSpec(t)
	if _, err := s.SetAcceptance([]string{"docs say"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Body, "- [x] The docs say so") {
		t.Errorf("substring selector missed:\n%s", s.Body)
	}

	s = newSpec(t)
	changed, err := s.SetAcceptance([]string{"all"}, true)
	if err != nil {
		t.Fatal(err)
	}
	// Only the two that were open changed — "all" is not "rewrite all".
	if len(changed) != 2 {
		t.Fatalf("changed %d criteria, want 2 (the third was already ticked)", len(changed))
	}
	if done, total := Progress(s.Body); done != total {
		t.Errorf("Progress = %d/%d, want everything ticked", done, total)
	}
}

func TestSetAcceptanceUnchecks(t *testing.T) {
	s := newSpec(t)
	if _, err := s.SetAcceptance([]string{"2"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Body, "- [ ] The CLI ticks a box") {
		t.Errorf("uncheck did not apply:\n%s", s.Body)
	}
}

func TestSetAcceptanceNoopWhenAlreadyInState(t *testing.T) {
	s := newSpec(t)
	before := s.Body
	changed, err := s.SetAcceptance([]string{"2"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 || s.Body != before {
		t.Errorf("ticking an already-ticked criterion must write nothing, changed=%+v", changed)
	}
}

func TestSetAcceptanceRefusesAmbiguousSelectors(t *testing.T) {
	cases := []struct {
		name, sel, want string
	}{
		{"out of range", "9", "no criterion 9"},
		{"unknown text", "nonsense", "no criterion matches"},
		{"ambiguous text", "The", "matches 3 criteria"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSpec(t)
			before := s.Body
			_, err := s.SetAcceptance([]string{tc.sel}, true)
			if err == nil {
				t.Fatalf("selector %q should have failed", tc.sel)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
			// Every error must show what there was to choose from.
			if !strings.Contains(err.Error(), "1. [ ] The board renders") {
				t.Errorf("error should carry the numbered checklist, got: %v", err)
			}
			if s.Body != before {
				t.Errorf("a failed selector must not half-apply:\n%s", s.Body)
			}
		})
	}
}

func TestSetAcceptanceResolvesEverythingBeforeWriting(t *testing.T) {
	s := newSpec(t)
	before := s.Body
	// The first selector is good, the second is not: nothing may be written.
	if _, err := s.SetAcceptance([]string{"1", "nonsense"}, true); err == nil {
		t.Fatal("expected the bad selector to fail the whole call")
	}
	if s.Body != before {
		t.Errorf("a partially valid call wrote anyway:\n%s", s.Body)
	}
}

func TestSetAcceptanceWithoutAChecklist(t *testing.T) {
	s := &Spec{ID: "tb-bare", Title: "Bare", Body: "## Goal\n\nNo checklist here.\n",
		File: filepath.Join(t.TempDir(), "tb-bare.md")}
	_, err := s.SetAcceptance([]string{"all"}, true)
	if err == nil || !strings.Contains(err.Error(), "no acceptance criteria") {
		t.Errorf("error = %v, want a complaint about the missing checklist", err)
	}
}

func TestCriteriaJoinsWrappedLines(t *testing.T) {
	wrapped := "## Acceptance\n\n" +
		"- [ ] The webhook tests pass under load\n" +
		"      and no test sleeps to avoid the race\n" +
		"- [x] The second one\n\n" +
		"Trailing prose, not indented, belongs to nobody.\n"
	cs := Criteria(wrapped)
	if len(cs) != 2 {
		t.Fatalf("got %d criteria, want 2: %+v", len(cs), cs)
	}
	want := "The webhook tests pass under load and no test sleeps to avoid the race"
	if cs[0].Text != want {
		t.Errorf("criterion 1 text = %q, want %q", cs[0].Text, want)
	}
	if cs[1].Text != "The second one" {
		t.Errorf("criterion 2 text = %q", cs[1].Text)
	}

	// A selector may name the wrapped half, and the tick still edits only
	// the checkbox line.
	s := &Spec{ID: "tb-wrap", Title: "Wrapped", Body: wrapped,
		File: filepath.Join(t.TempDir(), "tb-wrap.md")}
	if _, err := s.SetAcceptance([]string{"no test sleeps"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Body, "- [x] The webhook tests pass under load\n      and no test sleeps") {
		t.Errorf("the tick must land on the checkbox line only:\n%s", s.Body)
	}
}

// TestProofRoundTrips covers the syntax contract: evidence is written on
// the criterion's own line, read back off it, and a re-tick replaces the
// clause instead of stacking a second one.
func TestProofRoundTrips(t *testing.T) {
	cases := []struct {
		line string
		kind string
		ref  string
		text string
	}{
		{"the list writes itself — proof: `TestListGrows`", "test", "TestListGrows", "the list writes itself"},
		{"the report drops nothing -- proof: internal/report/report.go", "path", "internal/report/report.go", "the report drops nothing"},
		{"a bare filename counts — proof: `flow.go`", "path", "flow.go", "a bare filename counts"},
		{"the pipeline is green — proof: `ci:build`", "ci", "ci:build", "the pipeline is green"},
		{"a prose promise with an — em dash in it", "", "", "a prose promise with an — em dash in it"},
		{"nothing to prove", "", "", "nothing to prove"},
	}
	for _, tc := range cases {
		c := Criterion{Text: tc.line}
		ev, text := c.Proof()
		if ev.Kind != tc.kind || ev.Ref != tc.ref {
			t.Errorf("Proof(%q) = %+v, want kind %q ref %q", tc.line, ev, tc.kind, tc.ref)
		}
		if text != tc.text {
			t.Errorf("Proof(%q) left text %q, want %q", tc.line, text, tc.text)
		}
	}

	// Re-proving replaces, never stacks.
	once := WithProof("the thing is true", "TestOne")
	twice := WithProof(once, "TestTwo")
	if strings.Count(twice, "proof:") != 1 {
		t.Errorf("re-proving stacked clauses: %q", twice)
	}
	if !strings.Contains(twice, "TestTwo") || strings.Contains(twice, "TestOne") {
		t.Errorf("re-proving kept the wrong evidence: %q", twice)
	}
	if got := WithProof(once, ""); got != "the thing is true" {
		t.Errorf("clearing evidence left %q", got)
	}
}

// TestTickRecordsEvidenceOnTheLineItTouches: the point of ticking cheaply
// is that the diff reads as one promise coming true.
func TestTickRecordsEvidenceOnTheLineItTouches(t *testing.T) {
	dir := t.TempDir()
	s := &Spec{ID: "tb-p1", Title: "Proving", File: dir + "/tb-p1.md",
		Body: "## Acceptance\n\n- [ ] first promise\n- [ ] second promise\n"}

	changed, err := s.SetAcceptanceWithProof([]string{"1"}, true, "TestFirstPromise")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 {
		t.Fatalf("changed %d criteria, want 1", len(changed))
	}
	if !strings.Contains(s.Body, "- [x] first promise — proof: `TestFirstPromise`") {
		t.Errorf("body did not record the evidence:\n%s", s.Body)
	}
	if !strings.Contains(s.Body, "- [ ] second promise") {
		t.Errorf("an untouched criterion was rewritten:\n%s", s.Body)
	}

	// Evidence can be attached to a criterion that is already ticked: the
	// tick was the claim, the proof is what makes it re-derivable.
	if _, err := s.SetAcceptanceWithProof([]string{"1"}, true, "TestBetterName"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Body, "TestBetterName") || strings.Contains(s.Body, "TestFirstPromise") {
		t.Errorf("evidence was not updated on an already-ticked criterion:\n%s", s.Body)
	}

	// A bare tick still works and leaves evidence alone.
	if _, err := s.SetAcceptance([]string{"2"}, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.Body, "- [x] second promise") || !strings.Contains(s.Body, "TestBetterName") {
		t.Errorf("a bare tick disturbed the checklist:\n%s", s.Body)
	}
}
