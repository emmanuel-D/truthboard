package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// A tick is a claim. Evidence is what makes the claim checkable again.
//
// tb-a066 made ticking cheap and made a missing tick visible, which was the
// right first move and left one thing unfixed: what a tick *is*. Recorded
// once, it stays true in the file forever — including after the test that
// proved it was deleted. That is exactly where statuses stood before this
// tool existed, and statuses escaped it by pointing at evidence git can
// re-read. So can a tick.
//
// Every audit re-checks the evidence. A named test or path that is no
// longer there is drift — the claim outlived the thing that supported it —
// and drift is all it is: git alone still derives the status, and a done
// story with broken evidence still reads done.

// BrokenProof is a ticked criterion whose evidence is gone.
type BrokenProof struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Criterion string `json:"criterion"`
	N         int    `json:"n"`
	Ref       string `json:"ref"`
	Kind      string `json:"kind"`
	Why       string `json:"why"`
}

// UncheckedProof is evidence this checkout cannot speak to — a CI check
// that lives in a forge, not in the tree. Reported apart from broken
// evidence on purpose: "I looked and it is gone" and "I cannot look from
// here" are different facts, and collapsing them would either cry wolf
// about every pipeline name or quietly bless one.
type UncheckedProof struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Criterion string `json:"criterion"`
	N         int    `json:"n"`
	Ref       string `json:"ref"`
	Why       string `json:"why"`
}

// deriveProofs re-checks every ticked criterion that names its evidence.
// Criteria carrying none are untouched: evidence is opt-in, because prose
// promises are the reason acceptance is a human claim in the first place.
func deriveProofs(res *Result, specs []spec.Spec, repo string) {
	byID := make(map[string]*spec.Spec, len(specs))
	for i := range specs {
		byID[specs[i].ID] = &specs[i]
	}
	seen := newTreeIndex(repo)

	for i := range res.Specs {
		ss := &res.Specs[i]
		s := byID[ss.ID]
		if s == nil {
			continue
		}
		for _, c := range s.Acceptance() {
			ev, text := c.Proof()
			if ev.Ref == "" {
				continue
			}
			if !c.Checked {
				// Evidence on an unticked criterion is a plan, not a claim.
				continue
			}
			ss.AcceptanceProved++
			switch ev.Kind {
			case "ci":
				res.Drift.UncheckedProofs = append(res.Drift.UncheckedProofs, UncheckedProof{
					ID: ss.ID, Title: ss.Title, Criterion: text, N: c.N, Ref: ev.Ref,
					Why: "a forge check cannot be seen from a checkout — this one is taken on trust",
				})
			case "path":
				if !seen.path(ev.Ref) {
					res.Drift.BrokenProofs = append(res.Drift.BrokenProofs, BrokenProof{
						ID: ss.ID, Title: ss.Title, Criterion: text, N: c.N, Ref: ev.Ref, Kind: ev.Kind,
						Why: "the file it points at is not in the tree any more",
					})
				}
			default:
				if !seen.symbol(ev.Ref) {
					res.Drift.BrokenProofs = append(res.Drift.BrokenProofs, BrokenProof{
						ID: ss.ID, Title: ss.Title, Criterion: text, N: c.N, Ref: ev.Ref, Kind: ev.Kind,
						Why: "no file in the tree mentions that name any more",
					})
				}
			}
		}
	}
}

// treeIndex answers "is this still here" without paying for a git process
// per criterion. A repo where forty criteria name tests would otherwise
// spawn forty greps on every audit, and the board re-audits constantly.
type treeIndex struct {
	repo    string
	symbols map[string]bool
}

func newTreeIndex(repo string) *treeIndex {
	return &treeIndex{repo: repo, symbols: map[string]bool{}}
}

// path reports whether a declared file is still in the working tree. It is
// deliberately the tree and not the index: evidence names something a
// reader can open.
func (t *treeIndex) path(ref string) bool {
	clean := filepath.Clean(ref)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return false // evidence points inside the repo or nowhere
	}
	_, err := os.Stat(filepath.Join(t.repo, clean))
	return err == nil
}

// symbol reports whether a name still appears anywhere in the tracked tree.
//
// It is a weak check on purpose, and honest about being one: a name that
// appears nowhere cannot be the test that proved anything, while a name
// that does appear might be in a comment. Catching the deletion is the
// point — that is the failure evidence exists to survive — and a stricter
// per-language parse would be a language list to maintain and a new way to
// be wrong about somebody's toolchain.
func (t *treeIndex) symbol(ref string) bool {
	if hit, asked := t.symbols[ref]; asked {
		return hit
	}
	// The spec directory is excluded, and that exclusion is the whole
	// mechanism working: the criterion that names TestFoo *is* a tracked
	// line containing "TestFoo", so a search including intent would find
	// every reference in the file that declares it and never report a
	// deletion. Evidence has to live where the work lives.
	//
	// --untracked because the test that proves a criterion is usually
	// written in the same session as the tick, and telling someone their
	// brand-new test does not exist — until they commit — would teach them
	// the check is noise.
	//
	// -w so "TestFoo" does not match "TestFoobar"; --fixed-strings because
	// a test name is a literal, not a pattern someone wrote for us.
	_, found := gitrepo.Try(t.repo, "grep", "--quiet", "-w", "--fixed-strings", "--untracked", ref,
		"--", ":!"+spec.RelDir)
	t.symbols[ref] = found
	return found
}

// Summary reads as a sentence wherever the finding is printed.
func (b BrokenProof) Summary() string {
	return fmt.Sprintf("criterion %d claims %s — %s", b.N, b.Ref, b.Why)
}

func (u UncheckedProof) Summary() string {
	return fmt.Sprintf("criterion %d rests on %s — %s", u.N, u.Ref, u.Why)
}
