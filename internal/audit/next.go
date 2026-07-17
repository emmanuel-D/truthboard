package audit

// Next returns the story an idle agent (or human) should pick up: the
// first planned spec in backlog order — priority first, unset last, id as
// tie-break, the same order every board renders. Deterministic: the same
// repo state always yields the same answer, and the answer changes only
// when the repo does (the chosen story stops being planned the moment
// someone pushes a branch for it).
//
// Returns nil when nothing is planned, along with the count of stalled
// specs so callers can point at work worth resuming instead of inventing
// new work.
func Next(repo string) (*SpecStatus, int, error) {
	res, err := Audit(repo, Options{})
	if err != nil {
		return nil, 0, err
	}
	stalled := 0
	var next *SpecStatus
	for i := range res.Specs {
		switch res.Specs[i].Status {
		case Planned:
			if next == nil {
				next = &res.Specs[i]
			}
		case Stalled:
			stalled++
		}
	}
	return next, stalled, nil
}
