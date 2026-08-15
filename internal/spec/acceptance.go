package spec

// Acceptance criteria are the one promise a spec makes that git cannot
// check for it. Git proves a commit landed; it has no idea whether the
// thing the story asked for is true. So the tick is intent — a claim, like
// the hold note — and it stays a claim: nothing here derives, gates or
// changes a status, and a done spec with nothing ticked is still done.
//
// What this file exists for is cost. The only way to tick a box used to be
// rewriting the entire body, which is why landed stories kept arriving with
// an untouched checklist: recording the truth was more expensive than
// skipping it. Ticking one criterion should cost one criterion.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// checkboxPattern matches one markdown task item, capturing its state and
// its text. This is the single dialect — the board's progress counts and
// the tick verbs read the same lines, so they can never disagree.
var checkboxPattern = regexp.MustCompile(`^(\s*[-*] \[)([ xX])(\]\s*)(.*)$`)

// Criterion is one acceptance checkbox in a spec body.
type Criterion struct {
	N       int    `json:"n"` // 1-based, as printed and as selectors name it
	Checked bool   `json:"checked"`
	Text    string `json:"text"`
	line    int    // index into the body's lines; the only thing a tick touches
}

// Criteria returns the acceptance checklist found in a body, in order.
//
// A criterion wrapped over several lines reads as one criterion: markdown
// says so, and a half-sentence would be a poor thing to print in a report
// or to match a selector against. Only the checkbox line is ever edited.
func Criteria(body string) []Criterion {
	var out []Criterion
	for i, line := range strings.Split(body, "\n") {
		m := checkboxPattern.FindStringSubmatch(line)
		if m == nil {
			// A continuation line: indented, non-empty, and not a new item.
			if n := len(out); n > 0 && isContinuation(line) {
				out[n-1].Text = strings.TrimSpace(out[n-1].Text + " " + strings.TrimSpace(line))
			}
			continue
		}
		out = append(out, Criterion{
			N:       len(out) + 1,
			Checked: m[2] != " ",
			Text:    strings.TrimSpace(m[4]),
			line:    i,
		})
	}
	return out
}

var itemStart = regexp.MustCompile(`^\s*([-*+]|\d+[.)])\s`)

// isContinuation reports whether a line continues the criterion above it —
// indented body text, not a new list item and not a heading.
func isContinuation(line string) bool {
	if strings.TrimSpace(line) == "" || itemStart.MatchString(line) || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	return line != strings.TrimLeft(line, " \t")
}

// Progress counts ticked and total criteria in a body.
func Progress(body string) (done, total int) {
	for _, c := range Criteria(body) {
		total++
		if c.Checked {
			done++
		}
	}
	return done, total
}

// Acceptance returns this spec's checklist.
func (s *Spec) Acceptance() []Criterion { return Criteria(s.Body) }

// Checklist renders the criteria the way every prompt and error should show
// them: numbered, with their state, so the next selector is obvious.
func Checklist(cs []Criterion) string {
	var b strings.Builder
	for _, c := range cs {
		mark := " "
		if c.Checked {
			mark = "x"
		}
		ev, text := c.Proof()
		fmt.Fprintf(&b, "  %d. [%s] %s", c.N, mark, text)
		// The three states a reader must be able to tell apart: not claimed,
		// claimed, and claimed with something that can be checked again.
		if ev.Ref != "" {
			fmt.Fprintf(&b, "  (proof: %s)", ev.Ref)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// SetAcceptance ticks (or unticks) the criteria named by selectors and
// writes the file, returning the criteria whose state actually changed.
//
// A selector is a 1-based index, or a case-insensitive substring of one
// criterion's text, or "all". Anything that does not resolve to exactly one
// criterion is an error carrying the numbered checklist — a wrong guess
// must never half-apply, so selectors are resolved completely before a
// single byte is written.
//
// Only the matched checkbox markers are rewritten. Every other line of the
// body, including prose between criteria, survives byte for byte, which is
// what keeps the resulting git diff readable as "this promise came true".
func (s *Spec) SetAcceptance(selectors []string, checked bool) ([]Criterion, error) {
	return s.SetAcceptanceWithProof(selectors, checked, "")
}

// SetAcceptanceWithProof ticks criteria and records what proves them. An
// empty proof leaves whatever the line already carried, so re-ticking a
// criterion never silently drops the evidence someone attached to it.
func (s *Spec) SetAcceptanceWithProof(selectors []string, checked bool, proof string) ([]Criterion, error) {
	cs := s.Acceptance()
	if len(cs) == 0 {
		return nil, fmt.Errorf("%s has no acceptance criteria to tick — add a '## Acceptance' checklist first", s.ID)
	}
	if len(selectors) == 0 {
		return nil, fmt.Errorf("name at least one criterion (an index, a unique substring, or \"all\"):\n%s", Checklist(cs))
	}

	targets := map[int]bool{}
	for _, sel := range selectors {
		matched, err := resolve(cs, sel)
		if err != nil {
			return nil, err
		}
		for _, n := range matched {
			targets[n] = true
		}
	}

	lines := strings.Split(s.Body, "\n")
	var changed []Criterion
	for _, c := range cs {
		if !targets[c.N] {
			continue
		}
		// Recording evidence on an already-ticked criterion is a change
		// worth writing: the tick was the claim, the proof is what makes it
		// re-derivable later.
		writingProof := checked && proof != ""
		if c.Checked == checked && !writingProof {
			continue
		}
		mark := " "
		if checked {
			mark = "x"
		}
		m := checkboxPattern.FindStringSubmatch(lines[c.line])
		text := m[4]
		if writingProof {
			text = WithProof(text, proof)
		}
		if lines[c.line] == m[1]+mark+m[3]+text {
			continue // nothing to write; the line already says this
		}
		lines[c.line] = m[1] + mark + m[3] + text
		c.Checked = checked
		c.Text = strings.TrimSpace(text)
		changed = append(changed, c)
	}
	if len(changed) == 0 {
		return nil, nil // already in the asked-for state; writing would be noise
	}
	s.Body = strings.Join(lines, "\n")
	if err := s.Save(); err != nil {
		return nil, err
	}
	return changed, nil
}

// resolve turns one selector into the criteria numbers it names.
func resolve(cs []Criterion, sel string) ([]int, error) {
	sel = strings.TrimSpace(sel)
	if strings.EqualFold(sel, "all") {
		all := make([]int, 0, len(cs))
		for _, c := range cs {
			all = append(all, c.N)
		}
		return all, nil
	}
	if n, err := strconv.Atoi(sel); err == nil {
		if n < 1 || n > len(cs) {
			return nil, fmt.Errorf("no criterion %d — this spec has %d:\n%s", n, len(cs), Checklist(cs))
		}
		return []int{n}, nil
	}
	var hits []int
	for _, c := range cs {
		if strings.Contains(strings.ToLower(c.Text), strings.ToLower(sel)) {
			hits = append(hits, c.N)
		}
	}
	switch len(hits) {
	case 1:
		return hits, nil
	case 0:
		return nil, fmt.Errorf("no criterion matches %q:\n%s", sel, Checklist(cs))
	default:
		return nil, fmt.Errorf("%q matches %d criteria — name one by index:\n%s", sel, len(hits), Checklist(cs))
	}
}

// Evidence is what a tick points at so the claim can be re-derived rather
// than remembered.
//
// A tick says "this promise came true". Recorded once, it stays true in the
// file forever, including after the test that proved it was deleted — which
// is the same position statuses were in before this tool existed: someone
// asserts, everyone trusts, the assertion rots quietly. Statuses escaped it
// by pointing at evidence git can re-read on every audit, and a tick can do
// the same.
//
// The syntax is a trailing clause on the criterion's own line, written and
// read by hand:
//
//   - [x] the acceptance list writes itself — proof: `TestAcceptanceListGrows`
//   - [x] the report drops nothing — proof: `internal/report/report.go`
//   - [x] the pipeline is green — proof: `ci:build`
//
// Evidence stays optional on purpose. Prose criteria — "a PO can read this"
// — are the reason acceptance is a human claim in the first place, and a
// scheme that could only express machine-checkable promises would quietly
// push those out of the checklist.
type Evidence struct {
	Kind string `json:"kind"` // test | path | ci
	Ref  string `json:"ref"`
}

// proofPattern reads the trailing evidence clause off a criterion's text.
// An em dash or a double hyphen, because both are what people type.
var proofPattern = regexp.MustCompile(`\s*(?:—|--)\s*proof:\s*(.+?)\s*$`)

// Proof returns the criterion's evidence, and the text with the evidence
// clause removed — every renderer wants the promise and the proof apart.
// A criterion carrying none returns a zero Evidence, which is not an error:
// most criteria are prose and always will be.
func (c Criterion) Proof() (Evidence, string) {
	m := proofPattern.FindStringSubmatch(c.Text)
	if m == nil {
		return Evidence{}, c.Text
	}
	ref := strings.Trim(strings.TrimSpace(m[1]), "`")
	if ref == "" {
		return Evidence{}, c.Text
	}
	return Evidence{Kind: evidenceKind(ref), Ref: ref}, strings.TrimSpace(c.Text[:len(c.Text)-len(m[0])])
}

// evidenceKind types a reference by how it is written, so the audit knows
// how to go and check it. A "ci:" prefix names a check that lives in a
// forge and cannot be seen from a checkout at all; anything that looks like
// a filename is a path; the rest is a test name.
func evidenceKind(ref string) string {
	switch {
	case strings.HasPrefix(ref, "ci:"):
		return "ci"
	case strings.ContainsAny(ref, "/\\") || filepathLike(ref):
		return "path"
	default:
		return "test"
	}
}

// filepathLike reports whether a bare reference reads as a filename: a dot
// with an extension after it, and no spaces. "flow.go" is a path,
// "TestFlowIsDerived" is not.
func filepathLike(ref string) bool {
	i := strings.LastIndex(ref, ".")
	if i <= 0 || i == len(ref)-1 || strings.ContainsAny(ref, " \t") {
		return false
	}
	for _, r := range ref[i+1:] {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// WithProof returns the criterion line's text with an evidence clause
// attached, replacing any clause already there. Writing it back through one
// function is what keeps a re-tick from stacking two proofs on a line.
func WithProof(text, ref string) string {
	bare := proofPattern.ReplaceAllString(text, "")
	if strings.TrimSpace(ref) == "" {
		return bare
	}
	return fmt.Sprintf("%s — proof: `%s`", strings.TrimSpace(bare), strings.Trim(strings.TrimSpace(ref), "`"))
}
