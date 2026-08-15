// Package mirror publishes the board outward, to the place the people who
// do not run truthboard already are.
//
// The forge boundary has been one-way since forge enrichment was written:
// the board reads pull requests, checks and claims, and nothing ever goes
// back. So the tool serves whoever runs it and the agents it wires, and
// stops there — the reviewer on a pull request, the colleague who lives in
// Issues, the stakeholder with a browser and no Go toolchain all see
// nothing unless somebody keeps a shared board on a machine with a URL.
//
// One rule decides the whole design: the markdown files stay the source of
// truth. A mirrored issue is an *output*, like the digest — rewritable from
// the specs at any time, and never something the board derives from. That
// is why each issue says, in its own body, that it is a mirror and where
// the original lives: the failure mode of every sync tool ever written is
// somebody editing the copy.
package mirror

import (
	"fmt"
	"sort"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Plan is what mirroring would do, computed before anything is written. It
// is the whole of --dry-run, and also what Apply walks: showing and doing
// share one plan so the preview cannot drift from the act.
type Plan struct {
	Repo      string   `json:"repo"`
	Create    []Action `json:"create,omitempty"`
	Update    []Action `json:"update,omitempty"`
	Close     []Action `json:"close,omitempty"`
	Unchanged []string `json:"unchanged,omitempty"` // spec ids already mirrored and current
}

// Action is one issue to write.
type Action struct {
	SpecID string `json:"spec"`
	Number int    `json:"issue,omitempty"` // existing issue; 0 when it must be created
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Why    string `json:"why"`
}

// Empty reports whether mirroring would change nothing.
func (p *Plan) Empty() bool {
	return len(p.Create) == 0 && len(p.Update) == 0 && len(p.Close) == 0
}

// Summary is the one-line count, rendered once so the dry run and the real
// run report themselves identically.
func (p *Plan) Summary() string {
	if p.Empty() {
		return fmt.Sprintf("%s is already up to date — %d story(ies) mirrored", p.Repo, len(p.Unchanged))
	}
	return fmt.Sprintf("%s: %d to create, %d to update, %d to close (%d already current)",
		p.Repo, len(p.Create), len(p.Update), len(p.Close), len(p.Unchanged))
}

// titlePrefix is how a mirrored issue is recognised later, and the reason
// no mapping file exists: the spec id is in the issue's own title, so the
// link between a story and its issue is *derived* from the forge on every
// run. A mapping kept on disk would be one fresh clone away from opening a
// second copy of every issue.
func titlePrefix(id string) string { return id + ": " }

// Title is the issue title for a story.
func Title(id, title string) string { return titlePrefix(id) + title }

// Build works out what mirroring would do, from the derived board and the
// issues already on the forge. It writes nothing and talks to nothing.
func Build(repo string, specs []spec.Spec, res *audit.Result, existing []Issue) *Plan {
	p := &Plan{Repo: repo}

	byID := make(map[string]*spec.Spec, len(specs))
	for i := range specs {
		byID[specs[i].ID] = &specs[i]
	}
	mirrored := map[string]Issue{}
	for _, is := range existing {
		if id, ok := specIDOf(is.Title, byID); ok {
			// The oldest issue for an id wins: if a duplicate ever appeared,
			// converging on one of them beats opening a third.
			if prev, seen := mirrored[id]; !seen || is.Number < prev.Number {
				mirrored[id] = is
			}
		}
	}

	for _, ss := range res.Specs {
		s := byID[ss.ID]
		if s == nil {
			continue
		}
		title := Title(ss.ID, ss.Title)
		body := Body(s, ss)
		is, exists := mirrored[ss.ID]

		if !exists {
			// A story that is already done and has never been mirrored is
			// still worth publishing: the point is the record, and an issue
			// that arrives closed is a record.
			p.Create = append(p.Create, Action{SpecID: ss.ID, Title: title, Body: body,
				Why: "not mirrored yet"})
			continue
		}

		// Rewritten and closed are independent facts about one issue, and a
		// story that changed *and* landed needs both — deciding between them
		// would leave the forge needing a second run to converge.
		stale := is.Title != title || strings.TrimSpace(is.Body) != strings.TrimSpace(body)
		landed := ss.Status == audit.Done && is.State == "open"
		if stale {
			p.Update = append(p.Update, Action{SpecID: ss.ID, Number: is.Number, Title: title, Body: body,
				Why: "the story changed since it was mirrored"})
		}
		if landed {
			p.Close = append(p.Close, Action{SpecID: ss.ID, Number: is.Number, Title: title,
				Why: "the story derived done"})
		}
		if !stale && !landed {
			p.Unchanged = append(p.Unchanged, ss.ID)
		}
	}

	sort.Strings(p.Unchanged)
	return p
}

// specIDOf reads the story id back out of a mirrored issue's title.
//
// It resolves against the stories that actually exist rather than against a
// pattern for what an id looks like. Ids are generated as hex today and are
// hand-editable in frontmatter, so a shape rule would both miss real ones
// and risk claiming an issue somebody wrote themselves. "Is this prefix a
// story on this board" has neither problem.
func specIDOf(title string, known map[string]*spec.Spec) (string, bool) {
	id, _, found := strings.Cut(title, ": ")
	if !found || known[id] == nil {
		return "", false
	}
	return id, true
}

// Body renders the issue text: what the story promised, how much of it is
// signed off, and where the real thing lives.
//
// The status is stated as derived, with its evidence, precisely because a
// reader on the forge has no other way to know it was not typed by someone.
func Body(s *spec.Spec, ss audit.SpecStatus) string {
	var b strings.Builder

	// The mirror notice comes first, before anyone starts editing.
	fmt.Fprintf(&b, "> **Mirror of `%s`** — edit the story there, not here.\n", s.File)
	fmt.Fprintf(&b, "> This issue is rewritten from the repository; changes made in it are lost.\n\n")

	fmt.Fprintf(&b, "**Status: %s** — %s\n", ss.Status, ss.Evidence)
	fmt.Fprintf(&b, "_Derived from git. Nobody set it, and nothing here can._\n\n")

	if goal := section(s.Body, "Goal"); goal != "" {
		fmt.Fprintf(&b, "## Goal\n\n%s\n\n", goal)
	}

	cs := s.Acceptance()
	if len(cs) > 0 {
		fmt.Fprintf(&b, "## Acceptance — %d of %d signed off\n\n", ss.AcceptanceDone, ss.AcceptanceTotal)
		for _, c := range cs {
			mark := " "
			if c.Checked {
				mark = "x"
			}
			ev, text := c.Proof()
			fmt.Fprintf(&b, "- [%s] %s", mark, text)
			if ev.Ref != "" {
				fmt.Fprintf(&b, " _(proof: `%s`)_", ev.Ref)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	var facts []string
	if ss.Epic != "" {
		facts = append(facts, "epic `"+ss.Epic+"`")
	}
	if ss.Sprint != "" {
		facts = append(facts, "sprint `"+ss.Sprint+"`")
	}
	if ss.Owner != "" {
		facts = append(facts, "owner "+ss.Owner)
	}
	if len(ss.Branches) > 0 {
		facts = append(facts, "branch `"+strings.Join(ss.Branches, "`, `")+"`")
	}
	if len(facts) > 0 {
		fmt.Fprintf(&b, "%s\n", strings.Join(facts, " · "))
	}
	return b.String()
}

// section lifts one "## Heading" block out of a spec body.
func section(body, heading string) string {
	lines := strings.Split(body, "\n")
	var out []string
	in := false
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") {
			if in {
				break
			}
			in = strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(l, "## ")), heading)
			continue
		}
		if in {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// Apply writes the plan to the forge, in the order a reader would expect:
// create, then update, then close.
//
// It stops at the first failure and reports what it had already done. A
// half-mirrored forge is a real outcome — a token expires, a rate limit
// arrives mid-run — and the one unacceptable version of it is the silent
// one, where a command exits zero having published a third of the board.
func Apply(c Client, p *Plan) (done Applied, err error) {
	for _, a := range p.Create {
		n, e := c.Create(a.Title, a.Body)
		if e != nil {
			return done, fmt.Errorf("creating an issue for %s: %w", a.SpecID, e)
		}
		done.Created = append(done.Created, fmt.Sprintf("%s (#%d)", a.SpecID, n))
	}
	for _, a := range p.Update {
		if e := c.Update(a.Number, a.Title, a.Body); e != nil {
			return done, fmt.Errorf("updating issue #%d for %s: %w", a.Number, a.SpecID, e)
		}
		done.Updated = append(done.Updated, fmt.Sprintf("%s (#%d)", a.SpecID, a.Number))
	}
	for _, a := range p.Close {
		if e := c.Close(a.Number); e != nil {
			return done, fmt.Errorf("closing issue #%d for %s: %w", a.Number, a.SpecID, e)
		}
		done.Closed = append(done.Closed, fmt.Sprintf("%s (#%d)", a.SpecID, a.Number))
	}
	return done, nil
}

// Applied is what actually reached the forge — reported on success and on
// failure alike, so a half-finished run always says how far it got.
type Applied struct {
	Created []string `json:"created,omitempty"`
	Updated []string `json:"updated,omitempty"`
	Closed  []string `json:"closed,omitempty"`
}

func (a Applied) Summary() string {
	if len(a.Created) == 0 && len(a.Updated) == 0 && len(a.Closed) == 0 {
		return "nothing was written"
	}
	return fmt.Sprintf("%d created, %d updated, %d closed", len(a.Created), len(a.Updated), len(a.Closed))
}
