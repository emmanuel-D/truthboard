package report

import (
	"fmt"
	"io"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

// Summary renders the plain-language summary as markdown someone can paste
// into an email or a wiki page. Every word of judgement was already chosen
// in internal/audit — this only lays it out, so the board's panel and this
// document cannot drift apart in what they claim.
//
// withIDs is off by default on purpose: a reader who wants to look a story
// up should be able to, but identifiers in the default output are how a
// report stops being readable.
func Summary(w io.Writer, s *audit.Summary, withIDs bool) error {
	fmt.Fprintf(w, "# Where things stand — %s\n\n", s.Scope)
	fmt.Fprintf(w, "%s\n\n", s.Headline)

	section(w, "Delivered", s.Delivered, withIDs)
	section(w, "Broke after delivery", s.Broken, withIDs)
	section(w, "Being worked on", s.InFlight, withIDs)
	section(w, "Paused", s.Paused, withIDs)
	section(w, "Not started yet", s.NotStarted, withIDs)

	if s.Unestimated > 0 {
		verb, noun := "carry", "stories"
		if s.Unestimated == 1 {
			verb, noun = "carries", "story"
		}
		fmt.Fprintf(w, "_%d open %s %s no estimate, so it is not in the point totals — that is not the same as being zero-sized._\n",
			s.Unestimated, noun, verb)
	}
	return nil
}

func section(w io.Writer, heading string, items []audit.SummaryItem, withIDs bool) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(w, "## %s (%d)\n\n", heading, len(items))
	for _, it := range items {
		fmt.Fprintf(w, "- %s", it.Title)
		if it.Points > 0 {
			fmt.Fprintf(w, " (%d points)", it.Points)
		}
		if it.Reason != "" {
			fmt.Fprintf(w, " — %s", it.Reason)
		}
		if withIDs {
			fmt.Fprintf(w, " `%s`", it.ID)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}
