package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// draftPrompt asks for strict JSON so the answer can be written straight
// into spec files — the LLM proposes intent, the same files a human would
// write; statuses stay derived from git like everywhere else.
const draftPrompt = `You are drafting a product backlog for a git-native tracker.
Turn the concept below into one epic and 3-6 concrete stories.

Respond with ONLY a JSON object, no prose, no code fences:
{
  "epic": "kebab-case-slug",
  "stories": [
    {
      "title": "One line, outcome-shaped",
      "type": "story|bug|task",
      "priority": 1,
      "points": 3,
      "body": "## Goal\n\n(2-4 sentences: outcome and why)\n\n## Acceptance\n\n- [ ] **Given** X **when** Y **then** Z\n- [ ] ..."
    }
  ]
}

Rules: every story body MUST contain a "## Goal" section and a "## Acceptance"
checklist of Given/When/Then criteria. priority is 1 (now), 2 (next) or 3
(later). points is a small-integer estimate.

Concept: %s`

type draftedStory struct {
	Title    string `json:"title"`
	Type     string `json:"type"`
	Priority int    `json:"priority"`
	Points   int    `json:"points"`
	Body     string `json:"body"`
}

type draft struct {
	Epic    string         `json:"epic"`
	Stories []draftedStory `json:"stories"`
}

// Draft asks the provider to expand concept into stories and writes them as
// real spec files. It returns the created specs. A story that comes back
// without a goal or acceptance section is rejected — the working agreement
// bans placeholder specs, and that applies to machine authors too.
func Draft(p Provider, repo, concept, owner string) ([]*spec.Spec, error) {
	raw, err := p.Complete(fmt.Sprintf(draftPrompt, concept))
	if err != nil {
		return nil, err
	}
	var d draft
	if err := json.Unmarshal([]byte(stripFences(raw)), &d); err != nil {
		return nil, fmt.Errorf("%s returned unparseable JSON: %w\n%.400s", p.Name(), err, raw)
	}
	if len(d.Stories) == 0 {
		return nil, fmt.Errorf("%s proposed no stories", p.Name())
	}
	for i, st := range d.Stories {
		if strings.TrimSpace(st.Title) == "" {
			return nil, fmt.Errorf("story %d has no title", i+1)
		}
		if !strings.Contains(st.Body, "## Goal") || !strings.Contains(st.Body, "## Acceptance") {
			return nil, fmt.Errorf("story %q is missing a Goal or Acceptance section — refusing to write a placeholder spec", st.Title)
		}
		if !spec.ValidType(st.Type) {
			d.Stories[i].Type = "" // an invented type degrades to story rather than failing the batch
		}
	}
	var created []*spec.Spec
	for _, st := range d.Stories {
		s, err := spec.New(repo, strings.TrimSpace(st.Title), owner)
		if err != nil {
			return created, err
		}
		s.Body = strings.TrimSpace(st.Body)
		s.Epic = d.Epic
		s.Priority = clamp(st.Priority, 0, 3)
		s.Points = max(0, st.Points)
		s.Type = st.Type
		if err := s.Save(); err != nil {
			return created, err
		}
		created = append(created, s)
	}
	return created, nil
}

var fencePattern = regexp.MustCompile("(?s)^```[a-z]*\n(.*?)\n```\\s*$")

// stripFences tolerates a model that wraps its JSON in a code fence
// despite instructions — common enough to handle, cheap to remove.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if m := fencePattern.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
