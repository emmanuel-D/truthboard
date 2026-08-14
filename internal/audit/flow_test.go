package audit

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func flowStory(t *testing.T, res *Result, id string) StoryFlow {
	t.Helper()
	if res.Flow == nil {
		t.Fatal("no flow rollup on the result")
	}
	for _, s := range res.Flow.Stories {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("story %q not timed; stories = %+v, unmeasurable = %+v", id, res.Flow.Stories, res.Flow.Unmeasurable)
	return StoryFlow{}
}

func unmeasurableReason(res *Result, id string) string {
	if res.Flow == nil {
		return ""
	}
	for _, u := range res.Flow.Unmeasurable {
		if u.ID == id {
			return u.Reason
		}
	}
	return ""
}

func nearly(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.5 {
		t.Errorf("%s = %.2fh, want ~%.2fh", what, got, want)
	}
}

// TestCycleTimeRunsToTheMergeNotTheLastCommit is the measurement's whole
// point: work finished on a branch is not work delivered. A story whose
// branch sat unmerged for four days took those four days, and stopping the
// clock at the last commit written on the branch would delete every review
// queue from the numbers.
func TestCycleTimeRunsToTheMergeNotTheLastCommit(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))

	// Filed on day -12, worked on day -10, merged on day -6.
	f.commitContents("Story: measure me\n\nSpec: tb-cyc", now.AddDate(0, 0, -12),
		map[string]string{specFile("tb-cyc"): specBody("tb-cyc", "Measure me")})
	f.git("checkout", "-b", "feature/tb-cyc-work")
	f.commitContents("Build it\n\nSpec: tb-cyc", now.AddDate(0, 0, -10),
		map[string]string{"pkg/a.go": "package pkg"})
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -6), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-cyc-work'", "feature/tb-cyc-work")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := specByID(t, res, "tb-cyc").Status; got != Done {
		t.Fatalf("status = %q, want done", got)
	}

	s := flowStory(t, res, "tb-cyc")
	nearly(t, "cycle", s.CycleHours, 4*24) // day -10 → day -6
	nearly(t, "lead", s.LeadHours, 6*24)   // day -12 → day -6
	if s.Started != now.AddDate(0, 0, -10).Format("2006-01-02") {
		t.Errorf("started = %q, want the work commit's day", s.Started)
	}
	if s.Landed != now.AddDate(0, 0, -6).Format("2006-01-02") {
		t.Errorf("landed = %q, want the merge day", s.Landed)
	}
	if s.Filed != now.AddDate(0, 0, -12).Format("2006-01-02") {
		t.Errorf("filed = %q, want the day the story was written down", s.Filed)
	}

	// The aggregate says how many stories it speaks for, and over what.
	if res.Flow.Cycle.Stories != 1 {
		t.Errorf("cycle stories = %d, want 1", res.Flow.Cycle.Stories)
	}
	if res.Flow.Days != 90 || res.Flow.From == "" || res.Flow.To == "" {
		t.Errorf("window = %s..%s over %dd, want a stated 90-day window", res.Flow.From, res.Flow.To, res.Flow.Days)
	}
}

// TestFilingDoesNotStartTheCycleClock guards the distinction the two clocks
// exist for: writing a story down is not starting it. Only the lead time may
// count the days a story spent waiting in the backlog.
func TestFilingDoesNotStartTheCycleClock(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -40))

	f.commitContents("Story: sat in the backlog\n\nSpec: tb-wait", now.AddDate(0, 0, -30),
		map[string]string{specFile("tb-wait"): specBody("tb-wait", "Sat in the backlog")})
	f.commitContents("Deliver it\n\nSpec: tb-wait", now.AddDate(0, 0, -2),
		map[string]string{"pkg/b.go": "package pkg"})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	s := flowStory(t, res, "tb-wait")
	nearly(t, "cycle", s.CycleHours, 0) // one work commit, landed where it was written
	nearly(t, "lead", s.LeadHours, 28*24)
	if s.LeadHours <= s.CycleHours {
		t.Error("28 days in the backlog must show up in the lead time and nowhere else")
	}
}

// TestUnmeasurableStoriesAreNamedNotGuessed covers the honesty constraint:
// a story git cannot time is reported with the reason and excluded from the
// aggregates — never counted as zero, and never allowed to change a status.
func TestUnmeasurableStoriesAreNamedNotGuessed(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -20))

	// Linked by branch name alone: no commit ever carried the trailer — not
	// even the one that filed the story.
	f.commitContents("Story: no trailer anywhere", now.AddDate(0, 0, -15),
		map[string]string{specFile("tb-bare"): specBody("tb-bare", "No trailer anywhere")})
	f.git("checkout", "-b", "feature/tb-bare-work")
	f.commitContents("Work with no trailer", now.AddDate(0, 0, -14), map[string]string{"pkg/c.go": "package pkg"})
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -13), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-bare-work'", "feature/tb-bare-work")

	// Filed and merged, but every commit carrying the trailer only wrote the
	// story down — there is no work commit to start a clock from.
	f.commitContents("Story: filed only\n\nSpec: tb-filed", now.AddDate(0, 0, -12),
		map[string]string{specFile("tb-filed"): specBody("tb-filed", "Filed only")})
	f.git("checkout", "-b", "feature/tb-filed-work")
	f.commitContents("Reword the story\n\nSpec: tb-filed", now.AddDate(0, 0, -11),
		map[string]string{specFile("tb-filed"): specBody("tb-filed", "Filed only, reworded")})
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -10), "merge", "--no-ff", "-m", "Merge branch 'feature/tb-filed-work'", "feature/tb-filed-work")

	// One measurable story, so the aggregates have something to be wrong about.
	f.commitContents("Deliver it\n\nSpec: tb-real", now.AddDate(0, 0, -5),
		map[string]string{specFile("tb-real"): specBody("tb-real", "Real"), "pkg/d.go": "package pkg"})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"tb-bare", "tb-filed"} {
		if got := specByID(t, res, id).Status; got != Done {
			t.Errorf("%s = %q, want done — an unmeasurable story keeps the status git derived", id, got)
		}
		if unmeasurableReason(res, id) == "" {
			t.Errorf("%s must be reported as unmeasurable, with a reason", id)
		}
		for _, s := range res.Flow.Stories {
			if s.ID == id {
				t.Errorf("%s must not appear in the timed stories", id)
			}
		}
	}
	if r := unmeasurableReason(res, "tb-bare"); r == unmeasurableReason(res, "tb-filed") {
		t.Errorf("both stories gave the same reason %q — the reason must name what is actually missing", r)
	}
	// Excluded, not counted as zero: one story in, two left out.
	if res.Flow.Cycle.Stories != 1 {
		t.Errorf("cycle stories = %d, want 1 — unmeasurable stories must not enter the aggregate", res.Flow.Cycle.Stories)
	}
	if res.Flow.Cycle.MedianHours != flowStory(t, res, "tb-real").CycleHours {
		t.Error("the median moved: an unmeasurable story leaked into it as a zero")
	}
}

// TestNoNegativeDurationIsEverPublished pins the last guard: rewritten
// history can put a landing before the work that produced it, and the
// answer is to say so, not to publish a negative number.
func TestNoNegativeDurationIsEverPublished(t *testing.T) {
	now := time.Now()
	skewed := &specTimes{
		filed:     now.AddDate(0, 0, -3),
		started:   now.AddDate(0, 0, -1),
		landingAt: now.AddDate(0, 0, -2),
	}
	_, _, why := timeOf(skewed, "tb-skew")
	if why == "" {
		t.Fatal("a landing before its first work commit must be unmeasurable")
	}
	if !strings.Contains(why, "rewritten") {
		t.Errorf("reason = %q, want it to name the rewritten history", why)
	}
}

// TestThroughputCountsStoriesAndPoints checks the per-week and per-sprint
// arithmetic, including the rule points everywhere else already follow:
// unestimated stories are counted apart, never as zero points.
func TestThroughputCountsStoriesAndPoints(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -60))

	sprinted := func(id, title string, points int) string {
		s := "---\nid: " + id + "\ntitle: " + title + "\nsprint: s12\n"
		if points > 0 {
			s += "points: " + strconv.Itoa(points) + "\n"
		}
		return s + "---\n\n## Goal\nTest.\n"
	}
	// Two weeks apart, so they cannot land in the same bucket.
	f.commitContents("Deliver one\n\nSpec: tb-one", now.AddDate(0, 0, -21),
		map[string]string{specFile("tb-one"): sprinted("tb-one", "One", 3), "pkg/a.go": "package pkg"})
	f.commitContents("Deliver two\n\nSpec: tb-two", now.AddDate(0, 0, -7),
		map[string]string{specFile("tb-two"): sprinted("tb-two", "Two", 0), "pkg/b.go": "package pkg"})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	landed := 0
	weeksWithWork := 0
	for _, b := range res.Flow.Weeks {
		landed += b.Stories
		if b.Stories > 0 {
			weeksWithWork++
		}
	}
	if landed != 2 || weeksWithWork != 2 {
		t.Errorf("weekly throughput = %d stories over %d weeks, want 2 over 2: %+v", landed, weeksWithWork, res.Flow.Weeks)
	}
	if len(res.Flow.Weeks) < 12 {
		t.Errorf("got %d weekly buckets, want the whole window — a week nothing landed in is a fact", len(res.Flow.Weeks))
	}

	if len(res.Flow.Sprints) != 1 {
		t.Fatalf("sprint buckets = %+v, want one for s12", res.Flow.Sprints)
	}
	s := res.Flow.Sprints[0]
	if s.Label != "s12" || s.Stories != 2 || s.Points != 3 || s.Unestimated != 1 {
		t.Errorf("s12 = %+v, want 2 stories, 3 points, 1 unestimated", s)
	}
}

// TestWIPCountsWorkInFlight covers the series that makes a growing pile of
// half-finished work visible: a story counts from its first work commit
// until it lands, and one that never landed counts to the end.
func TestWIPCountsWorkInFlight(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))

	f.commitContents("Story: open\n\nSpec: tb-open", now.AddDate(0, 0, -20),
		map[string]string{specFile("tb-open"): specBody("tb-open", "Open")})
	f.git("checkout", "-b", "feature/tb-open-work")
	f.commitContents("Start it\n\nSpec: tb-open", now.AddDate(0, 0, -3),
		map[string]string{"pkg/open.go": "package pkg"})
	f.git("checkout", "main")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := specByID(t, res, "tb-open").Status; got == Done {
		t.Fatalf("status = %q, want unfinished work", got)
	}
	if res.Flow.OpenNow != 1 {
		t.Errorf("open now = %d, want 1", res.Flow.OpenNow)
	}
	if len(res.Flow.WIP) == 0 {
		t.Fatal("no WIP series")
	}
	last := res.Flow.WIP[len(res.Flow.WIP)-1]
	if last.Stories != 1 {
		t.Errorf("WIP at %s = %d, want the unlanded story counted", last.Date, last.Stories)
	}
	first := res.Flow.WIP[0]
	if first.Stories != 0 {
		t.Errorf("WIP at %s = %d, want 0 — the work had not started yet", first.Date, first.Stories)
	}
}

// TestFlowIsWindowedButSprintsAreNot covers the two windows living together:
// a story that landed before the window is out of the medians and the weekly
// buckets, and still counted in the sprint it belonged to.
func TestFlowIsWindowedButSprintsAreNot(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -200))

	old := "---\nid: tb-old\ntitle: Long ago\nsprint: s01\npoints: 5\n---\n\n## Goal\nTest.\n"
	f.commitContents("Deliver long ago\n\nSpec: tb-old", now.AddDate(0, 0, -150),
		map[string]string{specFile("tb-old"): old, "pkg/old.go": "package pkg"})
	f.commitContents("Deliver recently\n\nSpec: tb-new", now.AddDate(0, 0, -3),
		map[string]string{specFile("tb-new"): specBody("tb-new", "Recently"), "pkg/new.go": "package pkg"})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Flow.Cycle.Stories != 1 {
		t.Errorf("cycle stories = %d, want 1 — only what landed inside the window", res.Flow.Cycle.Stories)
	}
	for _, s := range res.Flow.Stories {
		if s.ID == "tb-old" {
			t.Error("a story that landed 150 days ago is outside the 90-day window")
		}
	}
	if len(res.Flow.Sprints) != 1 || res.Flow.Sprints[0].Label != "s01" || res.Flow.Sprints[0].Points != 5 {
		t.Errorf("sprint buckets = %+v, want s01 with its 5 points — a sprint is its own window", res.Flow.Sprints)
	}
}
