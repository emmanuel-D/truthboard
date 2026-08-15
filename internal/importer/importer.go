// Package importer brings an existing backlog in.
//
// Adoption assumed an empty one. `init --agents` wires a repository
// beautifully and then hands over a specs directory with nothing in it,
// while the team's actual work sits in GitHub Issues, Jira or Linear,
// hundreds of items deep. The honest options were retyping it or abandoning
// it, and both leave the board telling the truth about a fraction of the
// work — a tracker that only knows about the stories filed after Tuesday is
// not one anybody runs their week on.
//
// What makes this tractable is that import is a one-way, one-time move into
// files. Read the source, write the specs, commit them as intent. There is
// no sync, no live integration and no second source of truth: the moment
// the markdown exists, git derives everything and where it came from stops
// mattering. Anything that would keep the external tracker authoritative is
// deliberately absent.
//
// The interesting question is not the transport, it is fidelity — what an
// imported item *lacks*. Most have no acceptance criteria and no scope
// paths, so a naive import produces hundreds of stories that can never be
// verified. They arrive visibly incomplete rather than quietly fake.
package importer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Item is one row of somebody else's backlog, in the shape every supported
// source is read into.
type Item struct {
	Key      string   `json:"key"` // stable source identity, e.g. "github#412"
	Title    string   `json:"title"`
	Body     string   `json:"body,omitempty"`
	Owner    string   `json:"owner,omitempty"`
	Labels   []string `json:"labels,omitempty"`
	Priority int      `json:"priority,omitempty"`
	State    string   `json:"state,omitempty"` // open | closed — read only to skip the closed
	URL      string   `json:"url,omitempty"`
}

// Plan is what an import would write. It is the whole of --dry-run, and
// what Write walks, so the preview cannot drift from the act.
type Plan struct {
	Source  string   `json:"source"`
	New     []Item   `json:"new,omitempty"`
	Skipped []Skip   `json:"skipped,omitempty"`
	Mapping []string `json:"mapping"` // how fields were translated, stated rather than guessed
}

// Skip is an item the import deliberately left alone, and why. Every source
// row is accounted for: silently dropping half a backlog would be the worst
// possible outcome for a command somebody runs once.
type Skip struct {
	Key  string `json:"key"`
	Why  string `json:"why"`
	Spec string `json:"spec,omitempty"` // the story that already covers it
}

func (p *Plan) Summary() string {
	if len(p.New) == 0 {
		return fmt.Sprintf("%s: nothing new to import (%d item(s) already accounted for)", p.Source, len(p.Skipped))
	}
	return fmt.Sprintf("%s: %d story(ies) to write, %d skipped", p.Source, len(p.New), len(p.Skipped))
}

// Build decides what to import. It reads the existing specs to recognise
// what is already here and writes nothing.
//
// Recognition is by provenance, not by title: two people can file "fix the
// login bug" twice and mean it, while the same source item imported twice
// is always a duplicate.
func Build(source string, items []Item, existing []spec.Spec, includeClosed bool) *Plan {
	p := &Plan{Source: source, Mapping: mappingNotes()}

	already := map[string]string{}
	for _, s := range existing {
		if s.Imported != "" {
			already[s.Imported] = s.ID
		}
	}

	for _, it := range items {
		switch {
		case strings.TrimSpace(it.Title) == "":
			p.Skipped = append(p.Skipped, Skip{Key: it.Key, Why: "no title — there is no story to write"})
		case already[it.Key] != "":
			// Never overwritten: a story imported once belongs to whoever
			// has edited it since, and re-importing must not undo that.
			p.Skipped = append(p.Skipped, Skip{Key: it.Key, Why: "already imported", Spec: already[it.Key]})
		case !includeClosed && strings.EqualFold(it.State, "closed"):
			p.Skipped = append(p.Skipped, Skip{Key: it.Key, Why: "closed in the source — pass --closed to bring it anyway"})
		default:
			p.New = append(p.New, it)
		}
	}
	sort.SliceStable(p.New, func(i, j int) bool { return p.New[i].Key < p.New[j].Key })
	return p
}

// mappingNotes says how the fields were translated. Stated, because every
// one of these is a judgement someone else might have made differently, and
// a silent mapping is a surprise waiting in somebody's backlog.
func mappingNotes() []string {
	return []string{
		"title → title",
		"description/body → the story's Goal section, unchanged",
		"assignee → owner",
		"first label → epic (the rest are listed in the goal)",
		"priority P1/P2/P3, high/medium/low → 1/2/3; anything else is left unset rather than guessed",
		"state → not imported. Statuses are derived from git here, so an item the source called done starts as planned and becomes done when its commits land",
		"acceptance criteria → not present in any source, so imported stories carry none and say so",
	}
}

// Write creates the spec files. It returns the ids it wrote, in order.
//
// Each story is created through the same constructor the CLI uses, so
// imported ids are generated and collision-checked exactly like hand-filed
// ones — an import must not introduce a second kind of story.
func Write(repo string, p *Plan) ([]string, error) {
	var written []string
	for _, it := range p.New {
		s, err := spec.New(repo, it.Title, it.Owner)
		if err != nil {
			return written, fmt.Errorf("writing a story for %s: %w", it.Key, err)
		}
		s.Imported = it.Key
		s.Priority = it.Priority
		if len(it.Labels) > 0 {
			s.Epic = slug(it.Labels[0])
		}
		s.Body = body(it)
		if err := s.Save(); err != nil {
			return written, fmt.Errorf("writing %s: %w", s.File, err)
		}
		written = append(written, s.ID)
	}
	return written, nil
}

// body renders the story. The source text is carried across unchanged —
// paraphrasing somebody's backlog would be inventing intent — and the
// acceptance section says plainly that it is missing rather than shipping a
// placeholder criterion that would read as a real promise.
func body(it Item) string {
	var b strings.Builder
	b.WriteString("## Goal\n\n")
	if text := strings.TrimSpace(it.Body); text != "" {
		b.WriteString(text)
		b.WriteString("\n")
	} else {
		fmt.Fprintf(&b, "Imported from %s with no description.\n", it.Key)
	}
	if len(it.Labels) > 1 {
		fmt.Fprintf(&b, "\nOther labels on the source item: %s\n", strings.Join(it.Labels[1:], ", "))
	}
	if it.URL != "" {
		fmt.Fprintf(&b, "\nImported from %s (%s).\n", it.Key, it.URL)
	} else {
		fmt.Fprintf(&b, "\nImported from %s.\n", it.Key)
	}

	b.WriteString("\n## Acceptance\n\n")
	b.WriteString("_Not imported: no tracker exports acceptance criteria, so nobody has written\n")
	b.WriteString("this story's yet. Add them before anyone can verify it — until then this is a\n")
	b.WriteString("title and an intention, and the board says so by having nothing to check._\n")
	return b.String()
}

var slugUnsafe = strings.NewReplacer(" ", "-", "_", "-", "/", "-", ":", "-")

func slug(s string) string {
	return strings.ToLower(strings.Trim(slugUnsafe.Replace(strings.TrimSpace(s)), "-"))
}
