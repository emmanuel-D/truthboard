package audit

// A hold is the one field in Truthboard that a human writes and git cannot
// produce: why the work is paused. Deprioritised, waiting on legal, the
// vendor has not replied — no amount of history yields any of it.
//
// Which is exactly why it is dangerous. Every tracker that ever rotted did
// so because someone wrote a true sentence and nobody came back to delete
// it. So the note is intent, and whether it still holds is derived: git is
// allowed to contradict it. A hold on work that has landed, or on work that
// is visibly moving, is reported as contradicted everywhere the note
// appears — never repeated on its own as though it were current.
//
// The note can be wrong. It cannot be wrong silently.

// ContradictedHold is a hold note the evidence disagrees with: drift, in
// the same sense as a stale promise, and reported beside it.
type ContradictedHold struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Hold  string `json:"hold"` // what the human wrote
	Why   string `json:"why"`  // the evidence against it
}

// deriveHolds sets HoldContradicted on every spec whose hold note the
// evidence disagrees with, and collects them as drift. Runs after statuses
// and evidence are final — it reads git's verdict, it never changes it.
// A spec with no hold is untouched, so a repo that never writes one sees
// nothing new anywhere.
func deriveHolds(res *Result) {
	for i := range res.Specs {
		s := &res.Specs[i]
		if s.Hold == "" {
			continue
		}
		switch s.Status {
		case Done:
			// The landing evidence already reads as a sentence ("work
			// landed on main"), so prefixing it only stutters.
			s.HoldContradicted = "the work landed"
			if s.Evidence != "" {
				s.HoldContradicted = s.Evidence
			}
		case InProgress:
			s.HoldContradicted = "work is moving"
			if s.Evidence != "" {
				s.HoldContradicted += " — " + s.Evidence
			}
		default:
			// Stalled, planned, in-review and regressed are all consistent
			// with a pause: the note explains what the status cannot.
			continue
		}
		res.Drift.ContradictedHolds = append(res.Drift.ContradictedHolds, ContradictedHold{
			ID: s.ID, Title: s.Title, Hold: s.Hold, Why: s.HoldContradicted,
		})
	}
}
