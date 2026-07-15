// Package spec implements the intent layer from CONCEPT-V1: one markdown
// file per unit of work in .truthboard/specs/, YAML frontmatter + free-form
// body. A spec is the only thing a human (or agent) ever writes — status is
// never stored here, it is always derived from git.
package spec

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	ID     string   `yaml:"id" json:"id"`
	Title  string   `yaml:"title" json:"title"`
	Owner  string   `yaml:"owner,omitempty" json:"owner,omitempty"`
	Branch string   `yaml:"branch,omitempty" json:"branch,omitempty"` // glob, e.g. feature/tb-4f2a-*
	Paths  []string `yaml:"paths,omitempty" json:"paths,omitempty"`   // scope hint for future creep detection

	Body string `yaml:"-" json:"-"` // markdown below the frontmatter
	File string `yaml:"-" json:"-"`
}

// Trailer returns the commit trailer that links commits to this spec —
// the primary linking signal (branch globs are secondary).
func (s *Spec) Trailer() string { return "Spec: " + s.ID }

func Dir(repo string) string { return filepath.Join(repo, ".truthboard", "specs") }

var frontmatterPattern = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n?(.*)\z`)

// Load returns all specs in the repo, sorted by ID, or (nil, nil) when the
// repo has no spec directory — spec mode is strictly opt-in.
func Load(repo string) ([]Spec, error) {
	entries, err := os.ReadDir(Dir(repo))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var specs []Spec
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		s, err := parseFile(filepath.Join(Dir(repo), e.Name()))
		if err != nil {
			return nil, err
		}
		specs = append(specs, s)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs, nil
}

// Find returns the spec with the given ID.
func Find(repo, id string) (*Spec, error) {
	specs, err := Load(repo)
	if err != nil {
		return nil, err
	}
	for i := range specs {
		if specs[i].ID == id {
			return &specs[i], nil
		}
	}
	return nil, fmt.Errorf("no spec with id %q in %s", id, Dir(repo))
}

func parseFile(path string) (Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	m := frontmatterPattern.FindSubmatch(raw)
	if m == nil {
		return Spec{}, fmt.Errorf("%s: missing YAML frontmatter (--- ... ---)", path)
	}
	var s Spec
	if err := yaml.Unmarshal(m[1], &s); err != nil {
		return Spec{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.ID == "" || s.Title == "" {
		return Spec{}, fmt.Errorf("%s: frontmatter needs at least id and title", path)
	}
	s.Body = strings.TrimSpace(string(m[2]))
	s.File = path
	return s, nil
}

// New creates a spec file with a collision-resistant short ID and a
// suggested branch glob, returning the created spec.
func New(repo, title, owner string) (*Spec, error) {
	dir := Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	id, err := newID(dir, title)
	if err != nil {
		return nil, err
	}
	s := &Spec{
		ID:     id,
		Title:  title,
		Owner:  owner,
		Branch: fmt.Sprintf("*/%s-*", id),
		File:   filepath.Join(dir, fmt.Sprintf("%s-%s.md", id, slugify(title))),
	}
	s.Body = "## Goal\n\n(what outcome, and why)\n\n## Acceptance\n\n- [ ] (observable criterion)\n"
	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

// Save rewrites the spec file: frontmatter followed by the body.
func (s *Spec) Save() error {
	fm, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	content := fmt.Sprintf("---\n%s---\n\n%s\n", fm, strings.TrimSpace(s.Body))
	return os.WriteFile(s.File, []byte(content), 0o644)
}

// newID derives a short hash id (tb-xxxx) from the title and current time,
// lengthening on the rare collision with an existing file. Hash-based ids
// avoid the sequential-id merge collisions called out in CONCEPT-V1 §8.
func newID(dir, title string) (string, error) {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d", title, time.Now().UnixNano())
	sum := fmt.Sprintf("%016x", h.Sum64())
	for width := 4; width <= 16; width++ {
		id := "tb-" + sum[:width]
		matches, err := filepath.Glob(filepath.Join(dir, id+"-*.md"))
		if err != nil {
			return "", err
		}
		if len(matches) == 0 {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not derive a unique spec id in %s", dir)
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	slug := strings.Trim(slugPattern.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	if slug == "" {
		slug = "untitled"
	}
	return slug
}
