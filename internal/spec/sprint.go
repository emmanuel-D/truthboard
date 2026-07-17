package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SprintIntent is the optional intent file for a sprint slug:
// .truthboard/sprints/<slug>.md with start/end dates in the frontmatter.
// Like a spec it carries only intent — the sprint's state (future, active,
// completed) is derived from the dates at audit time, never stored.
type SprintIntent struct {
	Slug  string `yaml:"slug" json:"slug"`
	Start string `yaml:"start,omitempty" json:"start,omitempty"` // YYYY-MM-DD
	End   string `yaml:"end,omitempty" json:"end,omitempty"`     // YYYY-MM-DD, inclusive

	Body string `yaml:"-" json:"-"`
	File string `yaml:"-" json:"-"`
}

const DateLayout = "2006-01-02"

func SprintDir(repo string) string { return filepath.Join(repo, ".truthboard", "sprints") }

// LoadSprints returns all sprint intent files, sorted by slug, or (nil, nil)
// when the repo has none — sprint dates are opt-in on top of opt-in sprints.
func LoadSprints(repo string) ([]SprintIntent, error) {
	entries, err := os.ReadDir(SprintDir(repo))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var sprints []SprintIntent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(SprintDir(repo), e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		m := frontmatterPattern.FindSubmatch(raw)
		if m == nil {
			return nil, fmt.Errorf("%s: missing YAML frontmatter (--- ... ---)", path)
		}
		var s SprintIntent
		if err := yaml.Unmarshal(m[1], &s); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if s.Slug == "" {
			s.Slug = strings.TrimSuffix(e.Name(), ".md")
		}
		for _, d := range []string{s.Start, s.End} {
			if d == "" {
				continue
			}
			if _, err := time.Parse(DateLayout, d); err != nil {
				return nil, fmt.Errorf("%s: date %q is not YYYY-MM-DD", path, d)
			}
		}
		s.Body = strings.TrimSpace(string(m[2]))
		s.File = path
		sprints = append(sprints, s)
	}
	sort.Slice(sprints, func(i, j int) bool { return sprints[i].Slug < sprints[j].Slug })
	return sprints, nil
}
