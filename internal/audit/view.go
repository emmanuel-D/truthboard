package audit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Filter narrows what a board *shows*. It is the answer to a board that has
// outgrown the context of the agent reading it: at ninety-odd stories, a
// full board costs more tokens than the session that asked for it, and the
// first instruction of the working agreement — look before you work —
// becomes the most expensive call an agent makes.
//
// Nothing here derives anything. A filtered board and a full board report
// the same status for the same story; one simply says less. That separation
// is load-bearing: the moment a filter could change a status, "the status is
// whatever git says" would become "the status is whatever git says, given
// how you asked", which is the failure this tool exists to prevent.
type Filter struct {
	Status []Status  // any of these statuses; empty means all
	Epic   string    // exact epic slug
	Sprint string    // exact sprint slug
	Since  time.Time // stories whose last commit is on or after this date
	Limit  int       // keep at most this many, in the board order already computed
}

// Active reports whether the filter narrows anything at all.
func (f Filter) Active() bool {
	return len(f.Status) > 0 || f.Epic != "" || f.Sprint != "" || !f.Since.IsZero() || f.Limit > 0
}

// knownStatuses is every status a filter may name — the derived vocabulary,
// listed once so an error message can print exactly what is accepted.
var knownStatuses = []Status{Planned, InProgress, InReview, Stalled, Done, Regressed}

// ParseFilter turns wire values (MCP arguments, CLI flags) into a Filter,
// failing loudly on anything it does not understand. Silently ignoring an
// unknown status would be the worst outcome available: the caller would get
// a full board back and believe it was the narrow one they asked for.
func ParseFilter(status []string, epic, sprint, since string, limit int) (Filter, error) {
	f := Filter{Epic: epic, Sprint: sprint, Limit: limit}
	for _, s := range status {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		ok := false
		for _, known := range knownStatuses {
			if s == string(known) {
				f.Status, ok = append(f.Status, known), true
				break
			}
		}
		if !ok {
			return Filter{}, fmt.Errorf("unknown status %q — derived statuses are: %s", s, statusList())
		}
	}
	if since != "" {
		t, err := time.Parse(spec.DateLayout, since)
		if err != nil {
			return Filter{}, fmt.Errorf("invalid since %q — want a date like 2026-08-01", since)
		}
		f.Since = t
	}
	if limit < 0 {
		return Filter{}, fmt.Errorf("invalid limit %d — want a positive count, or 0 for no limit", limit)
	}
	return f, nil
}

func statusList() string {
	names := make([]string, len(knownStatuses))
	for i, s := range knownStatuses {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// Filtered returns a copy of the result carrying only the specs the filter
// keeps. The copy is shallow by design — units, drift and digest describe
// the repository, not the selection, and quietly trimming them to match a
// spec filter would let a narrow question hide a real finding.
func (r *Result) Filtered(f Filter) *Result {
	if r == nil || !f.Active() {
		return r
	}
	out := *r
	out.Specs = nil
	for _, s := range r.Specs {
		if !f.keeps(s) {
			continue
		}
		if f.Limit > 0 && len(out.Specs) == f.Limit {
			break
		}
		out.Specs = append(out.Specs, s)
	}
	return &out
}

func (f Filter) keeps(s SpecStatus) bool {
	if f.Epic != "" && s.Epic != f.Epic {
		return false
	}
	if f.Sprint != "" && s.Sprint != f.Sprint {
		return false
	}
	if len(f.Status) > 0 {
		match := false
		for _, want := range f.Status {
			if s.Status == want {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if !f.Since.IsZero() {
		// A story git has never seen has no date to compare, and answering
		// "changed since Monday" with a story nothing ever happened to would
		// be inventing activity. It is excluded, and `updated` says why.
		if s.Updated == "" {
			return false
		}
		when, err := time.Parse(spec.DateLayout, s.Updated)
		if err != nil || when.Before(f.Since) {
			return false
		}
	}
	return true
}

// Lean returns a copy of the result trimmed to what a reader with a context
// window needs: open stories in full, finished ones summarised, and the
// branch board reduced to the branches still doing something.
//
// The rule is "detail where a decision is still open". A story in flight
// needs its evidence, its blockers and its branches, because someone is
// about to act on it. A story that landed three months ago needs to be
// recognisable and countable — id, title, status, how much of its
// acceptance was ticked — and everything else about it is one get_brief
// away. Applied to this repo it is the difference between a board that fits
// in an agent's first call and one that is rejected before it is read.
func (r *Result) Lean() *Result {
	if r == nil {
		return r
	}
	out := *r

	// Three kinds of story, three answers.
	//
	// Work actually in flight — in progress, in review, stalled, regressed —
	// is carried whole and never capped. It is what the reader is about to
	// act on, and it is bounded by how much a team can genuinely have open
	// at once; a board that hid active work to save room would be answering
	// a different question than the one asked.
	//
	// A backlog is not bounded by anything. Three hundred planned stories is
	// an ordinary backlog, so planned work is summarised down to what decides
	// whether it is next, and only the top of the backlog is carried.
	//
	// Finished work is bounded by how long the project has been running,
	// which is the growth that broke this in the first place: the most
	// recently finished stories are summarised, the rest are counted. "What
	// landed in March" is a filter (status:done, since) and a search
	// (find_spec) of its own.
	var planned, done []SpecStatus
	out.Specs = make([]SpecStatus, 0, len(r.Specs))
	for _, s := range r.Specs {
		switch s.Status {
		case Done:
			done = append(done, s)
		case Planned:
			planned = append(planned, s)
		default:
			out.Specs = append(out.Specs, s)
		}
	}

	omitted := Omitted{How: leanHow}
	// r.Specs is already in backlog order, so this keeps the top of the
	// backlog — the stories anyone would pick up next.
	for i, s := range planned {
		if i == plannedSample {
			omitted.PlannedSpecs = len(planned) - plannedSample
			break
		}
		out.Specs = append(out.Specs, summarisePlanned(s))
	}
	sort.SliceStable(done, func(i, j int) bool { return done[i].Updated > done[j].Updated })
	for i, s := range done {
		if i == doneSample {
			omitted.DoneSpecs = len(done) - doneSample
			break
		}
		out.Specs = append(out.Specs, summarise(s))
	}

	// Landed branches are already named in drift.landed_not_deleted; carrying
	// their full unit records here as well was, measured on this repo, eleven
	// kilobytes of the payload saying the same thing twice. The name list is
	// itself capped below — a repo that never prunes accumulates them without
	// limit, and the count is what the reader acts on.
	out.Units = make([]Unit, 0, len(r.Units))
	for _, u := range r.Units {
		if u.Status == Done {
			omitted.MergedBranches++
			continue
		}
		out.Units = append(out.Units, u)
	}

	// Unverified acceptance names the story and the count. Quoting every
	// unticked criterion in full is the same mistake in a different section:
	// on this repo it was thirty-nine kilobytes of criteria text, all of it
	// one get_brief away, in a list whose purpose is to say which stories
	// need a second look.
	out.Drift = r.Drift
	if len(r.Drift.LandedNotDeleted) > findingSample {
		out.Drift.LandedNotDeleted = r.Drift.LandedNotDeleted[:findingSample]
		omitted.SpentBranches = len(r.Drift.LandedNotDeleted) - findingSample
	}
	out.Drift.UnverifiedAcceptance = make([]UnverifiedAcceptance, 0, len(r.Drift.UnverifiedAcceptance))
	for i, ua := range r.Drift.UnverifiedAcceptance {
		if i == findingSample {
			omitted.UnverifiedAcceptance = len(r.Drift.UnverifiedAcceptance) - findingSample
			break
		}
		ua.Unticked = nil
		out.Drift.UnverifiedAcceptance = append(out.Drift.UnverifiedAcceptance, ua)
	}

	if omitted.DoneSpecs > 0 || omitted.PlannedSpecs > 0 || omitted.MergedBranches > 0 ||
		omitted.UnverifiedAcceptance > 0 || omitted.SpentBranches > 0 {
		out.Omitted = &omitted
	}
	return &out
}

// Omitted says what the lean view left out and how to ask for it. It is not
// optional politeness: a board that quietly dropped seventy stories would be
// a worse failure than one too big to read, because the reader would not
// know to ask. Every number here is a count of something still derivable.
type Omitted struct {
	PlannedSpecs         int    `json:"planned_specs,omitempty"`
	DoneSpecs            int    `json:"done_specs,omitempty"`
	MergedBranches       int    `json:"merged_branches,omitempty"`
	SpentBranches        int    `json:"spent_branches,omitempty"`
	UnverifiedAcceptance int    `json:"unverified_acceptance,omitempty"`
	How                  string `json:"how"`
}

// BoardBudget is the size the default board is held to, in characters —
// roughly fifteen thousand tokens. It is a stated ceiling rather than a
// hope: a test asserts a 120-story board marshals under it, so a change
// that quietly reintroduces an unbounded section fails before it ships.
//
// The number comes from what went wrong. This repo's board reached 94,609
// characters at 97 stories and was refused by the client that asked for it,
// which made "check the board first" the one instruction an agent could not
// follow. Everything the default view now carries is bounded: open work by
// how much is genuinely in flight, finished work by doneSample, drift lists
// by findingSample, digest and shipped by the digest window, flow by its own
// sample.
const BoardBudget = 60_000

const (
	// doneSample is how many finished stories the default board carries.
	// Enough to see what just shipped; not so many that the board's size is
	// a function of how long the project has been running.
	doneSample = 25
	// plannedSample is how much of the backlog's top the board carries. A
	// backlog is a queue: what is at the front decides what happens next,
	// and the rest is a filter away.
	plannedSample = 40
	// findingSample bounds a repeating drift list the same way the terminal
	// already bounds it on screen.
	findingSample = 25

	leanHow = "summarised to fit a context window — ask for what was left out: " +
		"status/epic/sprint/since/limit narrow the board, full:true carries every field, " +
		"find_spec searches the backlog, get_brief returns one story whole"
)

// summarise keeps what identifies a finished story and what can be counted,
// and drops what only matters while a decision is open. Status and the tick
// counts stay because they are the two things a reader checks about landed
// work: did it land, and did anyone verify it.
func summarise(s SpecStatus) SpecStatus {
	return SpecStatus{
		ID: s.ID, Title: s.Title, Status: s.Status,
		Epic: s.Epic, Sprint: s.Sprint, Type: s.Type,
		Priority: s.Priority, Points: s.Points, Owner: s.Owner,
		Updated:         s.Updated,
		AcceptanceDone:  s.AcceptanceDone,
		AcceptanceTotal: s.AcceptanceTotal,
	}
}

// summarisePlanned keeps everything that decides whether a story is the one
// to pick up next — priority, points, epic, sprint, unmet needs, a hold note
// — and drops the fields that describe work which has not happened. A
// planned story's evidence is always "no matching branch or commit yet", its
// branch list is empty, and its file is one get_brief away.
func summarisePlanned(s SpecStatus) SpecStatus {
	s.Evidence, s.File = "", ""
	s.Branches, s.PerRepo = nil, nil
	s.Landed, s.LandedRepo = "", ""
	return s
}

// Match is one hit from Find: enough to recognise a story and decide
// whether it is the one you were about to file again.
type Match struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  Status `json:"status"`
	Epic    string `json:"epic,omitempty"`
	Sprint  string `json:"sprint,omitempty"`
	Updated string `json:"updated,omitempty"`
	Where   string `json:"where"` // which part of the story matched: title, id, epic, body
	File    string `json:"file"`
}

// Find answers the question an agent has to ask before every create_spec —
// "has this already been filed?" — without making it download the board to
// find out. Matching is case-insensitive substring over the id, title, epic
// and the story's own text; the body is searched but never returned, since
// the point is a cheap answer.
func Find(res *Result, specs []spec.Spec, query string, limit int) []Match {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	bodies := make(map[string]string, len(specs))
	for i := range specs {
		bodies[specs[i].ID] = strings.ToLower(specs[i].Body)
	}
	if limit <= 0 {
		limit = 20
	}

	// Every word must appear, not the phrase: someone asking "board context"
	// about a story titled "the board no longer fits in its context" is
	// asking the right question, and a contiguous-substring search would
	// tell them it had never been filed.
	terms := strings.Fields(q)

	var out []Match
	for _, s := range res.Specs {
		title, id, epic := strings.ToLower(s.Title), strings.ToLower(s.ID), strings.ToLower(s.Epic)
		where := ""
		switch {
		case hasAll(title, terms):
			where = "title"
		case hasAll(id, terms):
			where = "id"
		case epic != "" && hasAll(epic, terms):
			where = "epic"
		case hasAll(title+" "+id+" "+epic+" "+bodies[s.ID], terms):
			where = "body"
		default:
			continue
		}
		out = append(out, Match{
			ID: s.ID, Title: s.Title, Status: s.Status, Epic: s.Epic,
			Sprint: s.Sprint, Updated: s.Updated, Where: where, File: s.File,
		})
	}

	// A title hit is a stronger answer than a word buried in a goal, so the
	// most likely duplicate is read first.
	rank := map[string]int{"title": 0, "id": 1, "epic": 2, "body": 3}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Where] < rank[out[j].Where] })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// hasAll reports whether every term appears in the haystack.
func hasAll(haystack string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(haystack, t) {
			return false
		}
	}
	return len(terms) > 0
}
