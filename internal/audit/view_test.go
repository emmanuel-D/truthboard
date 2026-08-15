package audit

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// bigBoard builds a result the size of a real, long-running backlog: 120
// stories, most of them finished, each carrying the fields a real audit
// fills in. The point is the payload, so every string is realistic in
// length — a fixture of "t1", "t2" would pass any budget and prove nothing.
func bigBoard(n int) *Result {
	res := &Result{
		Repo: "/Users/someone/dev/workspaces/project", Integration: "origin/main",
		ElectedVia: "origin/HEAD", StaleDays: 7, DigestDays: 14,
		GeneratedAt: time.Date(2026, 8, 15, 9, 0, 0, 0, time.UTC),
	}
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("tb-%04x", i)
		s := SpecStatus{
			ID:    id,
			Title: fmt.Sprintf("A story with a title of the length people actually write, number %d", i),
			Owner: "emmanuel", Epic: "po-experience", Sprint: "s12", Type: "story",
			Priority: 1 + i%3, Points: i % 8,
			Status:          Done,
			Evidence:        "work landed on origin/main — but feature/tb-" + id + "-slug still has unmerged commits",
			Branches:        []string{"feature/" + id + "-a-branch-name-of-realistic-length"},
			Landed:          "9f1c2d3e4b5a69788796a5b4c3d2e1f009182736",
			File:            ".truthboard/specs/" + id + "-a-story-with-a-title-of-the-length.md",
			Updated:         fmt.Sprintf("2026-%02d-%02d", 1+i%8, 1+i%28),
			AcceptanceDone:  3,
			AcceptanceTotal: 5,
		}
		// A realistic slice of the board is still open.
		switch i % 12 {
		case 0:
			s.Status = Planned
			s.Evidence = "no matching branch or commit yet"
			s.Branches, s.Landed = nil, ""
		case 1:
			s.Status = InProgress
			s.Evidence = "feature/" + id + "-slug — active 1d ago, 4 commits ahead, 0 behind"
		}
		res.Specs = append(res.Specs, s)

		if s.Status == Done {
			res.Units = append(res.Units, Unit{
				Name: "feature/" + id + "-a-branch-name-of-realistic-length", Status: Done,
				Tip: "9f1c2d3e4b5a69788796a5b4c3d2e1f009182736", LastCommit: time.Now(),
				Evidence: "tip is ancestor of origin/main (merged)", SpecID: id, Local: true, Remote: true,
			})
			res.Drift.LandedNotDeleted = append(res.Drift.LandedNotDeleted,
				"feature/"+id+"-a-branch-name-of-realistic-length")
			res.Drift.UnverifiedAcceptance = append(res.Drift.UnverifiedAcceptance, UnverifiedAcceptance{
				ID: id, Title: s.Title, Done: 3, Total: 5,
				Unticked: []string{
					"a criterion whose text is as long as the ones people really write in a spec",
					"another criterion, equally unabbreviated, describing what must become true",
				},
			})
		}
	}
	return res
}

func size(t *testing.T, v any) int {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

// TestDefaultBoardStaysWithinBudget is the regression this whole story
// exists for. The board that broke was 94,609 characters at 97 stories and
// was refused by the client that asked for it.
func TestDefaultBoardStaysWithinBudget(t *testing.T) {
	full := bigBoard(120)
	lean := full.Lean()

	got := size(t, lean)
	if got > BoardBudget {
		t.Errorf("default board is %d chars, over the stated budget of %d", got, BoardBudget)
	}
	// A budget no realistic board could exceed would prove nothing about the
	// summarising: the full board must actually be over it.
	if before := size(t, full); before <= BoardBudget {
		t.Fatalf("fixture is too small to be a test: full board is %d chars, under the %d budget", before, BoardBudget)
	}

	// Bounded, not merely smaller: twice the backlog must not mean twice the
	// board. This is the property that failed — every story filed made the
	// first call of every agent session more expensive.
	small := size(t, bigBoard(120).Lean())
	large := size(t, bigBoard(400).Lean())
	if large > small*2 {
		t.Errorf("board grew from %d to %d chars when the backlog tripled — the default view is still unbounded", small, large)
	}
}

// TestLeanSaysWhatItLeftOut guards the line between summarising and hiding.
// A reader who cannot tell that seventy stories were dropped has been
// misled, which is worse than a board too big to read.
func TestLeanSaysWhatItLeftOut(t *testing.T) {
	lean := bigBoard(120).Lean()
	if lean.Omitted == nil {
		t.Fatal("a view that dropped finished stories must say so")
	}
	if lean.Omitted.DoneSpecs == 0 || lean.Omitted.MergedBranches == 0 {
		t.Errorf("omitted counts = %+v, want the dropped stories and branches counted", lean.Omitted)
	}
	for _, want := range []string{"find_spec", "get_brief", "full"} {
		if !strings.Contains(lean.Omitted.How, want) {
			t.Errorf("the way to ask for what was left out must name %q: %q", want, lean.Omitted.How)
		}
	}
	// Work in flight is never summarised away — it is what the reader acts
	// on, and it is bounded by what a team can genuinely have open.
	inFlight := 0
	for _, s := range lean.Specs {
		if s.Status != InProgress {
			continue
		}
		inFlight++
		if s.Evidence == "" || len(s.Branches) == 0 {
			t.Errorf("%s is in flight and lost its evidence — work in flight is carried whole", s.ID)
		}
	}
	if inFlight == 0 {
		t.Fatal("the fixture has no work in flight, so it cannot prove work in flight is kept")
	}
	// A planned story keeps what decides whether it is next, and drops the
	// description of work that has not happened.
	for _, s := range lean.Specs {
		if s.Status != Planned {
			continue
		}
		if s.Priority == 0 || s.Epic == "" {
			t.Errorf("planned %s lost the fields that order a backlog: %+v", s.ID, s)
		}
		if s.Evidence != "" {
			t.Errorf("planned %s carries evidence of work that has not happened: %q", s.ID, s.Evidence)
		}
		break
	}
	// A finished story stays recognisable and countable.
	for _, s := range lean.Specs {
		if s.Status != Done {
			continue
		}
		if s.Title == "" || s.AcceptanceTotal == 0 {
			t.Errorf("summarised story %s lost its title or tick counts: %+v", s.ID, s)
		}
		if s.Evidence != "" || len(s.Branches) > 0 {
			t.Errorf("summarised story %s still carries open-work detail: %+v", s.ID, s)
		}
		break
	}
}

// TestFilteringNeverDerives is the constraint that keeps a filter safe: the
// same story reports the same status however it was asked for.
func TestFilteringNeverDerives(t *testing.T) {
	full := bigBoard(60)
	byStatus := map[string]Status{}
	for _, s := range full.Specs {
		byStatus[s.ID] = s.Status
	}

	f, err := ParseFilter([]string{"planned", "in-progress"}, "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	got := full.Filtered(f)
	if len(got.Specs) == 0 || len(got.Specs) == len(full.Specs) {
		t.Fatalf("filter kept %d of %d specs — it narrowed nothing", len(got.Specs), len(full.Specs))
	}
	for _, s := range got.Specs {
		if s.Status != Planned && s.Status != InProgress {
			t.Errorf("%s is %s, which the filter did not ask for", s.ID, s.Status)
		}
		if s.Status != byStatus[s.ID] {
			t.Errorf("%s derived %s unfiltered and %s filtered — a filter must never change a status", s.ID, byStatus[s.ID], s.Status)
		}
	}
	// The repository-level findings describe the repo, not the selection: a
	// narrow question must not make drift disappear.
	if len(got.Drift.LandedNotDeleted) != len(full.Drift.LandedNotDeleted) {
		t.Error("filtering the spec list must not trim the drift report")
	}
}

func TestFilterNarrowsEveryWayItClaimsTo(t *testing.T) {
	res := &Result{Specs: []SpecStatus{
		{ID: "tb-a", Status: Done, Epic: "core", Sprint: "s12", Updated: "2026-08-10"},
		{ID: "tb-b", Status: Planned, Epic: "ui", Sprint: "s12", Updated: "2026-08-01"},
		{ID: "tb-c", Status: Planned, Epic: "core", Sprint: "s13", Updated: "2026-07-01"},
		{ID: "tb-d", Status: Planned, Epic: "core"}, // never touched by git
	}}

	kept := func(t *testing.T, f Filter) []string {
		t.Helper()
		var ids []string
		for _, s := range res.Filtered(f).Specs {
			ids = append(ids, s.ID)
		}
		return ids
	}

	f, _ := ParseFilter(nil, "core", "", "", 0)
	if got := kept(t, f); strings.Join(got, ",") != "tb-a,tb-c,tb-d" {
		t.Errorf("epic filter kept %v", got)
	}
	f, _ = ParseFilter(nil, "", "s12", "", 0)
	if got := kept(t, f); strings.Join(got, ",") != "tb-a,tb-b" {
		t.Errorf("sprint filter kept %v", got)
	}
	f, _ = ParseFilter(nil, "", "", "2026-08-01", 0)
	if got := kept(t, f); strings.Join(got, ",") != "tb-a,tb-b" {
		t.Errorf("since filter kept %v — a story git never saw has no activity to report", got)
	}
	f, _ = ParseFilter(nil, "", "", "", 2)
	if got := kept(t, f); len(got) != 2 {
		t.Errorf("limit kept %v", got)
	}
}

// TestBadFilterFailsLoudly covers the worst available outcome: a caller who
// asked for a narrow board, got the whole one, and could not tell.
func TestBadFilterFailsLoudly(t *testing.T) {
	if _, err := ParseFilter([]string{"shipped"}, "", "", "", 0); err == nil {
		t.Error("an unknown status must fail, never be ignored")
	} else if !strings.Contains(err.Error(), "planned") {
		t.Errorf("the error must list what is accepted: %v", err)
	}
	if _, err := ParseFilter(nil, "", "", "last tuesday", 0); err == nil {
		t.Error("an unparseable date must fail")
	}
	if _, err := ParseFilter(nil, "", "", "", -3); err == nil {
		t.Error("a negative limit must fail")
	}
	// Case and padding are input noise, not a different question.
	if f, err := ParseFilter([]string{" DONE "}, "", "", "", 0); err != nil || len(f.Status) != 1 || f.Status[0] != Done {
		t.Errorf("ParseFilter(%q) = %+v, %v", " DONE ", f, err)
	}
}

func TestFindAnswersWhetherItWasAlreadyFiled(t *testing.T) {
	res := &Result{Specs: []SpecStatus{
		{ID: "tb-aaa", Title: "Cycle time from commits", Status: Done, Epic: "flow"},
		{ID: "tb-bbb", Title: "Something unrelated", Status: Planned, Epic: "ui"},
		{ID: "tb-ccc", Title: "Another thing", Status: Planned, Epic: "ui"},
	}}
	specs := []spec.Spec{
		{ID: "tb-aaa", Body: "## Goal\nmeasure things"},
		{ID: "tb-bbb", Body: "## Goal\nthis one mentions cycle time deep in its goal"},
		{ID: "tb-ccc", Body: "## Goal\nnothing relevant"},
	}

	got := Find(res, specs, "CYCLE TIME", 0)
	if len(got) != 2 {
		t.Fatalf("matches = %+v, want the title hit and the body hit", got)
	}
	// A title hit is the likelier duplicate and must be read first.
	if got[0].ID != "tb-aaa" || got[0].Where != "title" {
		t.Errorf("first match = %+v, want the title match first", got[0])
	}
	if got[1].Where != "body" {
		t.Errorf("second match = %+v, want it labelled as a body hit", got[1])
	}
	if got[0].Status != Done {
		t.Errorf("a match must carry its derived status, got %q", got[0].Status)
	}
	if Find(res, specs, "nothing here matches this", 0) != nil {
		t.Error("no match must be no match, not an empty-ish answer")
	}
	if len(Find(res, specs, "e", 1)) != 1 {
		t.Error("limit must bound the answer")
	}

	// Every word must appear, not the phrase. Someone asking "time cycle"
	// about "Cycle time from commits" is asking the right question, and a
	// contiguous-substring search would answer "never filed".
	if got := Find(res, specs, "time cycle", 0); len(got) == 0 || got[0].ID != "tb-aaa" {
		t.Errorf("words out of order found %+v, want the title match", got)
	}
	// All of them, though: one word matching is not a match.
	if got := Find(res, specs, "cycle unicorn", 0); len(got) != 0 {
		t.Errorf("a term that appears nowhere must exclude the story, got %+v", got)
	}
}
