package audit

import "sort"

// SprintRollup groups spec statuses by their sprint slug. A sprint has no
// status of its own — done/total is arithmetic over the same derived
// statuses shown everywhere else, and a sprint is "finished" exactly when
// its stories are. Open stories carry their status so the rollup answers
// "what rolls over" without inventing a sprint clock.
type SprintRollup struct {
	Name  string       `json:"name"`
	Done  int          `json:"done"`
	Total int          `json:"total"`
	Open  []SprintOpen `json:"open,omitempty"` // everything not done, backlog order
}

type SprintOpen struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status Status `json:"status"`
}

// rollupSprints derives the per-sprint view from res.Specs (already in
// backlog order). Specs without a sprint don't appear — sprints are
// opt-in, and a repo that never uses them sees nothing new.
func rollupSprints(res *Result) {
	byName := map[string]*SprintRollup{}
	for _, s := range res.Specs {
		if s.Sprint == "" {
			continue
		}
		r := byName[s.Sprint]
		if r == nil {
			r = &SprintRollup{Name: s.Sprint}
			byName[s.Sprint] = r
		}
		r.Total++
		if s.Status == Done {
			r.Done++
		} else {
			r.Open = append(r.Open, SprintOpen{ID: s.ID, Title: s.Title, Status: s.Status})
		}
	}
	if len(byName) == 0 {
		return
	}
	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	// Newest sprint first under the usual naming schemes (s12, 2026-29):
	// longer slugs are later, ties break lexicographically descending.
	sort.Slice(names, func(i, j int) bool {
		if len(names[i]) != len(names[j]) {
			return len(names[i]) > len(names[j])
		}
		return names[i] > names[j]
	})
	for _, n := range names {
		res.Sprints = append(res.Sprints, *byName[n])
	}
}
