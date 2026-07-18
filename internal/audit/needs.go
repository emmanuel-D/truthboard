package audit

import (
	"fmt"
	"strings"
)

// deriveWaiting computes readiness from the needs graph. Waiting is not a
// new typed status — it is arithmetic over the same derived statuses: a
// spec waits exactly on those of its needs that are not done. The waiting
// note is appended to the evidence line so every surface that shows
// evidence (report, board card, TUI, brief) tells it without new plumbing.
// Dependency cycles are drift: someone wrote intent that can never become
// ready, and the board must say so loudly rather than skip it in silence.
func deriveWaiting(res *Result) {
	status := make(map[string]Status, len(res.Specs))
	for _, s := range res.Specs {
		status[s.ID] = s.Status
	}
	for i := range res.Specs {
		s := &res.Specs[i]
		for _, need := range s.Needs {
			st, known := status[need]
			if known && st == Done {
				continue
			}
			if !known {
				// A hand-edited file can reference an id that is gone;
				// treat it as unmet so the gap stays visible.
				s.Waiting = append(s.Waiting, need+"?")
				continue
			}
			s.Waiting = append(s.Waiting, need)
		}
		if len(s.Waiting) > 0 && s.Status == Planned {
			s.Evidence += " — waiting on " + strings.Join(s.Waiting, ", ")
		}
	}
	res.Drift.DependencyCycles = findCycles(res.Specs)
}

// findCycles reports each dependency cycle once, rendered as a chain
// ("tb-a → tb-b → tb-a"), via iterative DFS with three-color marking.
func findCycles(specs []SpecStatus) []string {
	needs := make(map[string][]string, len(specs))
	for _, s := range specs {
		needs[s.ID] = s.Needs
	}
	const (
		white = 0 // unvisited
		grey  = 1 // on the current path
		black = 2 // fully explored
	)
	color := map[string]int{}
	var cycles []string
	var path []string

	var visit func(id string)
	visit = func(id string) {
		color[id] = grey
		path = append(path, id)
		for _, need := range needs[id] {
			switch color[need] {
			case white:
				if _, exists := needs[need]; exists {
					visit(need)
				}
			case grey:
				// The cycle is the path segment from need to id, closed.
				start := 0
				for i, p := range path {
					if p == need {
						start = i
						break
					}
				}
				cycles = append(cycles, fmt.Sprintf("%s → %s", strings.Join(path[start:], " → "), need))
			}
		}
		path = path[:len(path)-1]
		color[id] = black
	}
	for _, s := range specs {
		if color[s.ID] == white {
			visit(s.ID)
		}
	}
	return cycles
}
