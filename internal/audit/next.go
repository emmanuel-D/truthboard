package audit

// Next returns the story an idle agent (or human) should pick up: the
// first planned spec in backlog order — priority first, unset last, id as
// tie-break, the same order every board renders. Deterministic: the same
// repo state always yields the same answer, and the answer changes only
// when the repo does (the chosen story stops being planned the moment
// someone pushes a branch for it).
//
// Stories whose declared needs have not all landed are skipped — an agent
// must never be handed work whose foundation doesn't exist yet; they come
// back as the waiting list so callers can say what the holdup is.
//
// Returns nil when nothing is startable, along with the count of stalled
// specs so callers can point at work worth resuming instead of inventing
// new work.
func Next(repo string) (next *SpecStatus, stalled int, waiting []SpecStatus, err error) {
	res, err := Audit(repo, Options{})
	if err != nil {
		return nil, 0, nil, err
	}
	for i := range res.Specs {
		switch res.Specs[i].Status {
		case Planned:
			if len(res.Specs[i].Waiting) > 0 {
				waiting = append(waiting, res.Specs[i])
				continue
			}
			if next == nil {
				next = &res.Specs[i]
			}
		case Stalled:
			stalled++
		}
	}
	return next, stalled, waiting, nil
}
