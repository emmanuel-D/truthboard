package audit

import (
	"fmt"
	"sort"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Flow is the time dimension of the board. Every other derivation here
// answers "where does this story stand now"; this one answers "how long did
// it take, how much lands per week, and how much is in flight at once" — and
// answers it from the same evidence, so there is still nothing to type and
// nothing to keep up to date.
//
// A tracker asks people to record cycle time, which is why tracker cycle
// time measures who remembered to drag the card. Here the clock starts at
// the first commit that carried the story's trailer and did more than write
// the story down, and stops when the integration branch got it. Both ends
// are commits; neither can be typed, backdated or forgotten.
//
// What this must never do is guess. A story whose history cannot support a
// measurement is named in Unmeasurable with the reason, and takes no part in
// any aggregate — an excluded story is visible, a story silently counted as
// zero would quietly drag every median down.
type Flow struct {
	// The window every windowed figure below covers, so a number can never
	// be read as "all time" when it is not.
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`

	Cycle Stat `json:"cycle"` // first work commit → landed
	Lead  Stat `json:"lead"`  // story filed → landed

	// Headline is the whole rollup in one sentence, rendered once here so
	// the terminal, the markdown, the TUI and the board — which cannot share
	// Go code — cannot end up phrasing the same numbers differently.
	Headline string `json:"headline"`

	// Stories is a sample, not the population: the most recent storySample
	// timed stories, newest first. Every aggregate above covers the whole
	// window and says how many stories it speaks for — Cycle.Stories is the
	// real count, and it can legitimately exceed len(Stories).
	//
	// The cap is not cosmetic. This rollup is carried in `get_board`, which
	// an agent reads at the start of every session; a repo with a hundred
	// landed stories would have paid twenty kilobytes for per-story detail
	// that no surface renders.
	Stories      []StoryFlow  `json:"stories,omitempty"`
	Unmeasurable []Unmeasured `json:"unmeasurable,omitempty"` // landed, but git cannot time them
	Weeks        []Bucket     `json:"weeks,omitempty"`        // throughput per ISO week, oldest first
	Sprints      []Bucket     `json:"sprints,omitempty"`      // throughput per sprint slug, over all history
	WIP          []WIPPoint   `json:"wip,omitempty"`          // stories in flight, sampled weekly
	OpenNow      int          `json:"open_now"`               // stories started and not landed yet
}

// storySample is how many per-story timings travel with the rollup. The
// aggregates are computed over every story in the window regardless.
const storySample = 20

// StoryFlow is one landed story, timed. Dates are for reading; the hours
// are what the aggregates are computed from, because a story that took six
// hours and a story that took six days both "landed on the 3rd".
type StoryFlow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Epic   string `json:"epic,omitempty"`
	Sprint string `json:"sprint,omitempty"`
	Points int    `json:"points,omitempty"`

	Filed   string `json:"filed,omitempty"` // first commit carrying the trailer — usually the one that wrote the story down
	Started string `json:"started"`         // first commit carrying it that changed more than intent
	Landed  string `json:"landed"`          // reached the integration branch

	CycleHours float64 `json:"cycle_hours"`
	LeadHours  float64 `json:"lead_hours,omitempty"`
}

// Unmeasured is a story that landed and cannot be timed, with the reason
// stated in the terms of the evidence that is missing.
type Unmeasured struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// Stat summarises a set of durations. The median leads, not the mean: one
// story that sat in review over a holiday must not redraw the picture of a
// team's week. Stories is carried everywhere the figures are, so three
// stories can never be mistaken for thirty.
type Stat struct {
	Stories     int     `json:"stories"`
	MedianHours float64 `json:"median_hours"`
	P85Hours    float64 `json:"p85_hours"`
	MinHours    float64 `json:"min_hours"`
	MaxHours    float64 `json:"max_hours"`
}

// Bucket is what landed inside one window — a week, or a sprint. Points are
// summed only over estimated stories; Unestimated says how many are not in
// that sum, so the figure is never mistaken for the whole bucket.
type Bucket struct {
	Label       string `json:"label"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Stories     int    `json:"stories"`
	Points      int    `json:"points,omitempty"`
	Unestimated int    `json:"unestimated,omitempty"`
}

// WIPPoint is how many stories were in flight at one instant — started and
// not yet landed. Sampled weekly, at the end of each week in the window.
type WIPPoint struct {
	Date    string `json:"date"`
	Stories int    `json:"stories"`
}

// Empty reports whether a stat has nothing behind it. Renderers ask this
// rather than testing Stories directly, so "no data" reads the same way on
// every surface.
func (s Stat) Empty() bool { return s.Stories == 0 }

// Measured reports whether the flow found anything worth showing. A repo
// that adopted truthboard last week has a Flow with a window and no
// figures, and every surface must show the window rather than a zero.
func (f *Flow) Measured() bool {
	return f != nil && (len(f.Stories) > 0 || len(f.Unmeasurable) > 0 || f.OpenNow > 0)
}

// Duration renders hours the way a human reads an elapsed time: minutes
// under the hour, hours under the day, days after that. One place, so the
// terminal, the markdown and the board can never disagree about what 36.4
// means.
func Duration(hours float64) string {
	switch {
	case hours*60 < 1:
		return "under a minute"
	case hours < 1:
		return fmt.Sprintf("%dm", int(hours*60+0.5))
	case hours < 48:
		return fmt.Sprintf("%.1fh", hours)
	default:
		return fmt.Sprintf("%.1fd", hours/24)
	}
}

// deriveFlow times every landed story from the trailer index and rolls the
// results up. It reads what linkSpecs already derived and writes nothing
// back: no status is set, gated or downgraded here, and a story that cannot
// be timed keeps whatever status git gave it.
func deriveFlow(res *Result, idx *trailers, opts Options) {
	if idx == nil || len(res.Specs) == 0 {
		return
	}
	now := opts.Now
	from := now.AddDate(0, 0, -opts.FlowDays)
	f := &Flow{
		From: from.Format(spec.DateLayout),
		To:   now.Format(spec.DateLayout),
		Days: opts.FlowDays,
	}

	var all []timedStory
	// spans feed the WIP series, and include the stories no duration can
	// see: the ones that started and have not landed.
	var spans []flowSpan

	for i := range res.Specs {
		ss := &res.Specs[i]
		t := idx.times[ss.ID]

		if ss.Status != Done {
			// Not landed: it still counts as work in flight, from whichever
			// commit first carried its trailer on a branch.
			if t != nil && !t.startedAt().IsZero() {
				spans = append(spans, flowSpan{start: t.startedAt()})
				f.OpenNow++
			}
			continue
		}

		start, landed, why := timeOf(t, ss.ID)
		if why != "" {
			f.Unmeasurable = append(f.Unmeasurable, Unmeasured{ss.ID, ss.Title, why})
			continue
		}
		sf := StoryFlow{
			ID: ss.ID, Title: ss.Title, Epic: ss.Epic, Sprint: ss.Sprint, Points: ss.Points,
			Started:    start.Format(spec.DateLayout),
			Landed:     landed.Format(spec.DateLayout),
			CycleHours: landed.Sub(start).Hours(),
		}
		if !t.filed.IsZero() && !t.filed.After(landed) {
			sf.Filed = t.filed.Format(spec.DateLayout)
			sf.LeadHours = landed.Sub(t.filed).Hours()
		}
		all = append(all, timedStory{StoryFlow: sf, landed: landed})
		spans = append(spans, flowSpan{start: start, landed: landed})
	}

	// Newest first, matching every other list on the board.
	sort.Slice(all, func(i, j int) bool {
		if !all[i].landed.Equal(all[j].landed) {
			return all[i].landed.After(all[j].landed)
		}
		return all[i].ID < all[j].ID
	})

	var cycle, lead []float64
	for _, s := range all {
		if s.landed.Before(from) {
			continue // outside the window: still counted per sprint, below
		}
		if len(f.Stories) < storySample {
			f.Stories = append(f.Stories, s.StoryFlow)
		}
		cycle = append(cycle, s.CycleHours)
		if s.LeadHours > 0 {
			lead = append(lead, s.LeadHours)
		}
	}
	f.Cycle, f.Lead = stat(cycle), stat(lead)

	// Weeks cover the window; sprints cover all of history, because a sprint
	// is already a window somebody declared and truncating it to ninety days
	// would report a partial sprint as a whole one.
	buckets := make([]bucketable, 0, len(all))
	for _, s := range all {
		buckets = append(buckets, bucketable{
			landed: s.landed, sprint: s.Sprint, points: s.Points, unestimated: s.Points == 0,
		})
	}
	f.Weeks = weekBuckets(buckets, from, now)
	f.Sprints = sprintBuckets(buckets, res.Sprints)
	f.WIP = wipSeries(spans, from, now)

	sort.Slice(f.Unmeasurable, func(i, j int) bool { return f.Unmeasurable[i].ID < f.Unmeasurable[j].ID })
	f.Headline = f.headline()
	res.Flow = f
}

// PerWeek is how many stories landed per week over the window. Reported to
// one decimal because rounding 1.4 to "1 a week" is the kind of tidying
// that turns a measurement into a slogan.
func (f *Flow) PerWeek() float64 {
	if f == nil || f.Days <= 0 {
		return 0
	}
	return float64(f.Cycle.Stories) / (float64(f.Days) / 7)
}

// headline says what was measured, over what, and how many stories are
// behind it — in that order, so the sample size can never be read off
// separately from the figure it qualifies.
func (f *Flow) headline() string {
	if f.Cycle.Empty() {
		if len(f.Unmeasurable) > 0 {
			return fmt.Sprintf("nothing in the last %d days can be timed — %d landed stories carry no usable history",
				f.Days, len(f.Unmeasurable))
		}
		if f.OpenNow > 0 {
			return fmt.Sprintf("nothing landed in the last %d days · %d in flight", f.Days, f.OpenNow)
		}
		return fmt.Sprintf("nothing landed in the last %d days", f.Days)
	}
	s := fmt.Sprintf("cycle time %s median, %s at the 85th (%d stories in %d days) · %.1f landed/week",
		Duration(f.Cycle.MedianHours), Duration(f.Cycle.P85Hours), f.Cycle.Stories, f.Days, f.PerWeek())
	if !f.Lead.Empty() {
		s += fmt.Sprintf(" · %s median from filed to landed", Duration(f.Lead.MedianHours))
	}
	if f.OpenNow > 0 {
		s += fmt.Sprintf(" · %d in flight", f.OpenNow)
	}
	if n := len(f.Unmeasurable); n > 0 {
		s += fmt.Sprintf(" · %d not timeable", n)
	}
	return s
}

// timeOf reads one landed story's clock, or explains why it has none. The
// reasons are phrased in terms of the evidence that is missing, because the
// fix is always to that evidence: a story linked by branch name alone
// becomes measurable the moment a commit carries its trailer.
//
// It is the one place a duration can be rejected, so it is also the
// guarantee that no negative or invented duration reaches an aggregate.
func timeOf(t *specTimes, id string) (start, landed time.Time, why string) {
	switch {
	case t == nil || (t.filed.IsZero() && t.started.IsZero() && t.openSince.IsZero()):
		return start, landed, "no commit carries Spec: " + id + " — it is linked by branch name alone"
	case t.startedAt().IsZero():
		return start, landed, "only the commit that wrote the story down carries its trailer — no work commit to start the clock"
	case t.landedAt().IsZero():
		return start, landed, "no commit carrying its trailer is on the integration branch — the landing is proved by a merged branch, which has no moment"
	case t.landedAt().Before(t.startedAt()):
		return start, landed, fmt.Sprintf("it landed (%s) before its first work commit (%s) — the history was rewritten",
			t.landedAt().Format(spec.DateLayout), t.startedAt().Format(spec.DateLayout))
	}
	return t.startedAt(), t.landedAt(), ""
}

// timedStory is a measured story with the landing kept as a time, which the
// rollups need and the rendered StoryFlow has already turned into a date.
type timedStory struct {
	StoryFlow
	landed time.Time
}

// flowSpan is a story's occupancy of the board: when work on it began, and
// when it landed. A zero landing means it is still in flight.
type flowSpan struct {
	start  time.Time
	landed time.Time
}

// bucketable is the little of a story that a throughput rollup needs.
type bucketable struct {
	landed      time.Time
	sprint      string
	points      int
	unestimated bool
}

// stat computes the summary of a set of durations. An empty set yields an
// empty stat rather than a row of zeros — nothing measured must never
// render as "zero hours".
func stat(hours []float64) Stat {
	if len(hours) == 0 {
		return Stat{}
	}
	sorted := append([]float64(nil), hours...)
	sort.Float64s(sorted)
	return Stat{
		Stories:     len(sorted),
		MedianHours: percentile(sorted, 0.5),
		P85Hours:    percentile(sorted, 0.85),
		MinHours:    sorted[0],
		MaxHours:    sorted[len(sorted)-1],
	}
}

// percentile is the nearest-rank percentile of an already-sorted slice.
// Nearest-rank, not interpolated: every figure it reports is a duration
// some story actually took.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p*float64(len(sorted))+0.999999) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

// weekBuckets counts what landed in each ISO week of the window, oldest
// first. Empty weeks are kept: a week nothing landed in is a fact about the
// flow, and dropping it would draw a smooth line through a stall.
func weekBuckets(in []bucketable, from, to time.Time) []Bucket {
	if !from.Before(to) {
		return nil
	}
	byWeek := map[string]*Bucket{}
	var order []string
	for cur := weekStart(from); !cur.After(to); cur = cur.AddDate(0, 0, 7) {
		y, w := cur.ISOWeek()
		label := fmt.Sprintf("%d-W%02d", y, w)
		if byWeek[label] == nil {
			end := cur.AddDate(0, 0, 6)
			byWeek[label] = &Bucket{Label: label,
				Start: cur.Format(spec.DateLayout), End: end.Format(spec.DateLayout)}
			order = append(order, label)
		}
	}
	for _, s := range in {
		if s.landed.Before(from) || s.landed.After(to) {
			continue
		}
		y, w := s.landed.ISOWeek()
		b := byWeek[fmt.Sprintf("%d-W%02d", y, w)]
		if b == nil {
			continue
		}
		add(b, s)
	}
	out := make([]Bucket, 0, len(order))
	for _, l := range order {
		out = append(out, *byWeek[l])
	}
	return out
}

// sprintBuckets counts what landed per sprint slug. Only stories that
// declare a sprint appear — sprints are opt-in everywhere else, and a
// bucket of everything unsprinted would be a fiction with a name.
func sprintBuckets(in []bucketable, sprints []SprintRollup) []Bucket {
	byName := map[string]*Bucket{}
	for _, s := range in {
		if s.sprint == "" {
			continue
		}
		b := byName[s.sprint]
		if b == nil {
			b = &Bucket{Label: s.sprint}
			for _, sp := range sprints {
				if sp.Name == s.sprint {
					b.Start, b.End = sp.Start, sp.End
					break
				}
			}
			byName[s.sprint] = b
		}
		add(b, s)
	}
	if len(byName) == 0 {
		return nil
	}
	out := make([]Bucket, 0, len(byName))
	for _, b := range byName {
		out = append(out, *b)
	}
	// Same order sprints are shown in everywhere else: newest first.
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Label) != len(out[j].Label) {
			return len(out[i].Label) > len(out[j].Label)
		}
		return out[i].Label > out[j].Label
	})
	return out
}

func add(b *Bucket, s bucketable) {
	b.Stories++
	if s.unestimated {
		b.Unestimated++
	} else {
		b.Points += s.points
	}
}

// wipSeries samples how many stories were in flight at the end of each week
// of the window. A story counts from its first work commit until it lands;
// one that has not landed counts to the end of the series, which is what
// makes a growing pile of half-finished work visible.
func wipSeries(spans []flowSpan, from, to time.Time) []WIPPoint {
	if !from.Before(to) {
		return nil
	}
	var out []WIPPoint
	for cur := weekStart(from).AddDate(0, 0, 6); !cur.After(to.AddDate(0, 0, 6)); cur = cur.AddDate(0, 0, 7) {
		at := cur
		if at.After(to) {
			at = to
		}
		n := 0
		for _, s := range spans {
			if s.start.After(at) {
				continue
			}
			if s.landed.IsZero() || s.landed.After(at) {
				n++
			}
		}
		out = append(out, WIPPoint{Date: at.Format(spec.DateLayout), Stories: n})
	}
	return out
}

// weekStart is the Monday of the week containing t, in t's own location.
func weekStart(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	offset := (int(d.Weekday()) + 6) % 7 // Monday = 0
	return d.AddDate(0, 0, -offset)
}
