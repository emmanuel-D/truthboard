package audit

import "sort"

// NextUp is the answer to "what should I work on": the story to start, plus
// the two things an agent must hear before starting anything — what it is
// waiting on, and what it left unfinished behind it.
type NextUp struct {
	Spec    *SpecStatus  // the story to pick up; nil when nothing is startable
	Stalled int          // stalled specs, so a caller can point at work worth resuming
	Waiting []SpecStatus // planned specs whose needs have not all landed

	// Landed stories whose acceptance was never ticked, newest landing
	// first. Handed back with every answer because the moment an agent asks
	// for new work is the last moment anyone will remember the old work —
	// a warning here costs one line and closes the loop the agent left open.
	Unverified []UnverifiedAcceptance
}

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
// Spec is nil when nothing is startable, and the stalled count comes back
// regardless, so callers point at work worth resuming instead of inventing
// new work.
func Next(repo string) (*NextUp, error) {
	res, err := Audit(repo, Options{})
	if err != nil {
		return nil, err
	}
	var up NextUp
	for i := range res.Specs {
		switch res.Specs[i].Status {
		case Planned:
			if len(res.Specs[i].Waiting) > 0 {
				up.Waiting = append(up.Waiting, res.Specs[i])
				continue
			}
			if up.Spec == nil {
				up.Spec = &res.Specs[i]
			}
		case Stalled:
			up.Stalled++
		}
	}
	up.Unverified = byNewestLanding(res)
	return &up, nil
}

// byNewestLanding orders unverified acceptance by when the work landed, so
// the story an agent just finished is the one it hears about first. Only
// landings inside the digest window carry a date; older ones sort last,
// keeping the order stable rather than arbitrary.
func byNewestLanding(res *Result) []UnverifiedAcceptance {
	if len(res.Drift.UnverifiedAcceptance) == 0 {
		return nil
	}
	landed := make(map[string]string, len(res.Shipped))
	for _, s := range res.Shipped {
		landed[s.ID] = s.Date
	}
	out := append([]UnverifiedAcceptance(nil), res.Drift.UnverifiedAcceptance...)
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := landed[out[i].ID], landed[out[j].ID]
		if di != dj {
			return di > dj // ISO dates sort lexically; undated ("") sorts last
		}
		return out[i].ID < out[j].ID
	})
	return out
}
