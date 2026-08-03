package audit

import (
	"fmt"
	"strings"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Everything else Truthboard renders is written for the person who wrote
// the commits: stale promises, shadow work, unmerged, branch names before
// outcomes. This is the one surface written for the person who did not.
//
// Two rules make it that surface, and both are enforced here rather than in
// any renderer, so there is exactly one place where words are chosen:
//
//  1. No derived-status vocabulary and no identifiers. A story was
//     "delivered", not "done"; it is "paused", not "stalled". Ids and
//     branch names appear only when someone asks for them.
//  2. Nothing is asserted that the audit did not derive. A pause always
//     carries a reason, and the reason is whichever of three sources has
//     the most standing — never a guess.
type Summary struct {
	Scope    string `json:"scope"`            // "Sprint s12, 27 July – 7 August" or "the last 14 days"
	Sprint   string `json:"sprint,omitempty"` // the slug when a sprint was named; empty means the digest window
	Headline string `json:"headline"`         // one sentence a person can read aloud

	Delivered  []SummaryItem `json:"delivered,omitempty"`
	Broken     []SummaryItem `json:"broken,omitempty"` // delivered, then undone
	InFlight   []SummaryItem `json:"in_flight,omitempty"`
	Paused     []SummaryItem `json:"paused,omitempty"`
	NotStarted []SummaryItem `json:"not_started,omitempty"`

	PointsDelivered int `json:"points_delivered"`
	PointsTotal     int `json:"points_total"`
	Unestimated     int `json:"unestimated,omitempty"`
}

// SummaryItem is one story said plainly. ID is carried so a reader who
// wants to look something up can be given it on request, never by default.
type SummaryItem struct {
	Title  string `json:"title"`
	ID     string `json:"id"`
	Points int    `json:"points,omitempty"`
	Reason string `json:"reason,omitempty"` // why it is paused
}

// summariseAll attaches the digest-window summary to the result, so the
// board and `--format json` carry it without anyone asking and without an
// API key. A repo with nothing to say simply has no summary.
func summariseAll(res *Result) {
	if s, err := res.Summarise(""); err == nil {
		res.Summary = s
	}
}

// Summarise builds the plain-language summary. An empty sprint slug covers
// the digest window, mirroring how review behaves; a slug covers that
// sprint's stories.
func (r *Result) Summarise(sprint string) (*Summary, error) {
	s := &Summary{Scope: fmt.Sprintf("the last %d days", r.DigestDays), Sprint: sprint}
	if sprint != "" {
		s.Scope = "Sprint " + sprint
		for _, sp := range r.Sprints {
			if sp.Name == sprint && sp.Start != "" && sp.End != "" {
				s.Scope += ", " + humanRange(sp.Start, sp.End)
			}
		}
	}

	// Delivered: inside a sprint that is every story that landed. Over the
	// digest window it is what actually landed in it — a story delivered
	// last quarter is not news, and saying so would inflate the number.
	shipped := map[string]bool{}
	for _, sh := range r.Shipped {
		shipped[sh.ID] = true
	}

	for _, spec := range r.Specs {
		if sprint != "" && spec.Sprint != sprint {
			continue
		}
		item := SummaryItem{Title: spec.Title, ID: spec.ID, Points: spec.Points}
		switch spec.Status {
		case Done:
			if sprint == "" && !shipped[spec.ID] {
				continue // landed before this window opened
			}
			s.Delivered = append(s.Delivered, item)
			s.PointsDelivered += spec.Points
		case Regressed:
			s.Broken = append(s.Broken, item)
		case Stalled:
			item.Reason = r.pauseReason(spec)
			s.Paused = append(s.Paused, item)
		case Planned:
			// A planned story with a live hold is paused on purpose, not
			// merely unstarted — the distinction is the whole point of a
			// summary someone acts on.
			if reason := r.pauseReason(spec); reason != "" {
				item.Reason = reason
				s.Paused = append(s.Paused, item)
				break
			}
			s.NotStarted = append(s.NotStarted, item)
		default: // in-progress, in-review
			s.InFlight = append(s.InFlight, item)
		}
		if spec.Points > 0 {
			s.PointsTotal += spec.Points
		} else if spec.Status != Done {
			s.Unestimated++
		}
	}

	if len(s.Delivered)+len(s.InFlight)+len(s.Paused)+len(s.NotStarted)+len(s.Broken) == 0 {
		return nil, fmt.Errorf("nothing to summarise in %s", s.Scope)
	}
	s.Headline = headline(s)
	return s, nil
}

// pauseReason answers "why?" with whichever source has the most standing:
// a human's note first, then a dependency the audit can name, and only
// then the bare fact that nothing has happened. A contradicted hold is
// skipped entirely — git has already disproved it, and repeating it here
// would launder a stale reason into a report for people who cannot check.
func (r *Result) pauseReason(s SpecStatus) string {
	if s.Hold != "" && s.HoldContradicted == "" {
		return s.Hold
	}
	if len(s.Waiting) > 0 {
		var titles []string
		for _, id := range s.Waiting {
			titles = append(titles, r.titleOf(id))
		}
		return "waiting for " + joinWords(titles)
	}
	if days := r.quietDays(s); days > 0 {
		return fmt.Sprintf("nothing has happened for %s", plural(days, "day"))
	}
	if s.Hold != "" {
		// The only note we have was contradicted, so say that plainly
		// rather than either repeating it or claiming to know nothing.
		return "no current reason on record — the note we have is out of date"
	}
	return ""
}

// titleOf turns a spec id into the words a reader recognises. An id that
// names nothing is returned as-is: better a bare id than a silent gap.
func (r *Result) titleOf(id string) string {
	for _, s := range r.Specs {
		if s.ID == strings.TrimSuffix(id, "?") {
			return s.Title
		}
	}
	return id
}

// quietDays is how long the story's most recent branch has been untouched.
func (r *Result) quietDays(s SpecStatus) int {
	var newest time.Time
	for _, label := range s.Branches {
		for _, u := range r.Units {
			if u.Label() == label && u.LastCommit.After(newest) {
				newest = u.LastCommit
			}
		}
	}
	if newest.IsZero() {
		return 0
	}
	at := r.GeneratedAt
	if at.IsZero() {
		at = time.Now()
	}
	if d := int(at.Sub(newest).Hours() / 24); d > 0 {
		return d
	}
	return 0
}

func headline(s *Summary) string {
	var b strings.Builder
	if len(s.Delivered) == 0 {
		b.WriteString("Nothing was delivered in " + lowerFirst(s.Scope))
	} else {
		fmt.Fprintf(&b, "%s delivered in %s", plural(len(s.Delivered), "story"), lowerFirst(s.Scope))
		// "13 of 21 points" is a commitment met. Over a rolling window
		// there is no commitment to measure against — the denominator would
		// just be the whole backlog, which says nothing about the fortnight.
		switch {
		case s.Sprint != "" && s.PointsTotal > 0:
			fmt.Fprintf(&b, " — %d of %d points", s.PointsDelivered, s.PointsTotal)
		case s.PointsDelivered > 0:
			fmt.Fprintf(&b, " — %s", plural(s.PointsDelivered, "point"))
		}
	}
	if n := len(s.Paused); n > 0 {
		fmt.Fprintf(&b, ", %s paused", plural(n, "story"))
	}
	if n := len(s.Broken); n > 0 {
		fmt.Fprintf(&b, ", %s broke after delivery", plural(n, "story"))
	}
	b.WriteString(".")
	return b.String()
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	if word == "story" {
		return fmt.Sprintf("%d stories", n)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func joinWords(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func lowerFirst(s string) string {
	if s == "" || s[0] < 'A' || s[0] > 'Z' {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// humanRange turns two ISO dates into something readable aloud:
// "27 July – 7 August". Unparseable dates fall back to what was written.
func humanRange(start, end string) string {
	a, err1 := time.Parse(spec.DateLayout, start)
	b, err2 := time.Parse(spec.DateLayout, end)
	if err1 != nil || err2 != nil {
		return start + " – " + end
	}
	if a.Month() == b.Month() && a.Year() == b.Year() {
		return fmt.Sprintf("%d–%d %s", a.Day(), b.Day(), b.Month())
	}
	return fmt.Sprintf("%d %s – %d %s", a.Day(), a.Month(), b.Day(), b.Month())
}
