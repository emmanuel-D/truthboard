package audit

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// Since answers "what changed on the board while I was away" — the question
// a standup asks and the one a board has never been able to answer, because
// every status it serves describes now and nothing remembers before.
//
// It is derived, like everything else. No snapshot is stored, no state file
// is kept, and nothing has to have been running: the board as it stood at
// any commit is recomputed from that commit, and the difference is read off
// the two. Two people running it against the same repo with the same
// argument get the same answer, whether or not either of them was here.
//
// What that can honestly cover is bounded by what a commit remembers.
// Delivery, filing, retirement and acceptance ticks are all facts about
// commits and files, so they are recoverable at any point in history.
// "In progress" is not: it is a statement about branches that exist right
// now, and a branch deleted last week left nothing behind to say it was
// moving. Diff reports what git can prove and names the rest as out of
// reach rather than guessing at it.
type Diff struct {
	From     string `json:"from"`      // the ref as given
	FromSHA  string `json:"from_sha"`  // what it resolved to
	FromDate string `json:"from_date"` // when that commit landed
	To       string `json:"to"`
	ToSHA    string `json:"to_sha"`
	ToDate   string `json:"to_date"`

	Landed   []Change `json:"landed,omitempty"`   // reached the integration branch
	Unlanded []Change `json:"unlanded,omitempty"` // had landed then, does not now — a revert
	Filed    []Change `json:"filed,omitempty"`    // written down since
	Retired  []Change `json:"retired,omitempty"`  // intent file deleted since
	Ticked   []Change `json:"ticked,omitempty"`   // acceptance criteria signed off
	Unticked []Change `json:"unticked,omitempty"` // a criterion that stopped being true

	// Verified and Unverified are the one drift signal a commit can prove at
	// both ends: landed work whose acceptance nobody read back.
	Verified   []Change `json:"verified,omitempty"`   // drift closed: landed work fully ticked since
	Unverified []Change `json:"unverified,omitempty"` // drift open: landed since, criteria still unticked
}

// Change is one thing that happened to one story.
type Change struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

// Quiet reports whether nothing happened. A quiet window is an answer, not
// an empty result — and the reason a scheduled digest can stay silent
// instead of posting "nothing to report" every morning.
func (d *Diff) Quiet() bool {
	return d != nil && len(d.Landed) == 0 && len(d.Unlanded) == 0 && len(d.Filed) == 0 &&
		len(d.Retired) == 0 && len(d.Ticked) == 0 && len(d.Unticked) == 0 &&
		len(d.Verified) == 0 && len(d.Unverified) == 0
}

// Headline says what happened in one sentence, rendered once here so the
// terminal and the webhook cannot describe the same window differently.
func (d *Diff) Headline() string {
	if d.Quiet() {
		return fmt.Sprintf("nothing changed on the board between %s and %s", d.FromDate, d.ToDate)
	}
	var parts []string
	add := func(n int, one, many string) {
		if n == 1 {
			parts = append(parts, "1 "+one)
		} else if n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(len(d.Landed), "story landed", "stories landed")
	add(len(d.Unlanded), "story came undone", "stories came undone")
	add(len(d.Filed), "story filed", "stories filed")
	add(len(d.Retired), "story retired", "stories retired")
	add(len(d.Ticked), "story signed off further", "stories signed off further")
	add(len(d.Unticked), "criterion withdrawn", "stories had criteria withdrawn")
	add(len(d.Unverified), "landed story unverified", "landed stories unverified")
	return fmt.Sprintf("%s → %s: %s", d.FromDate, d.ToDate, strings.Join(parts, ", "))
}

// snapshot is the board as some commit knew it: which stories existed, what
// they promised, and which had been delivered by then.
type snapshot struct {
	sha    string
	date   time.Time
	titles map[string]string
	done   map[string]int // ticked criteria
	total  map[string]int
	landed map[string]bool
}

func (s *snapshot) ids() []string {
	out := make([]string, 0, len(s.titles))
	for id := range s.titles {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// SinceRef resolves what the user typed — a ref, a sha, or a date — into a
// commit on the integration branch. A date resolves to the last commit on
// or before it, because "what changed since Monday" means since the state
// the repo was actually in on Monday.
func SinceRef(repo, base, want string) (string, error) {
	if want == "" {
		return "", fmt.Errorf("since what? give a ref (origin/main@{yesterday}), a commit, or a date (2026-08-01)")
	}
	if _, err := time.Parse(spec.DateLayout, want); err == nil {
		sha, ok := gitrepo.Try(repo, "rev-list", "-1", "--before="+want+" 00:00:00", base)
		if !ok || strings.TrimSpace(sha) == "" {
			return "", fmt.Errorf("no commit on %s before %s — the history does not reach back that far", base, want)
		}
		return strings.TrimSpace(sha), nil
	}
	sha, ok := gitrepo.Try(repo, "rev-parse", "--verify", want+"^{commit}")
	if !ok || strings.TrimSpace(sha) == "" {
		return "", fmt.Errorf("cannot resolve %q to a commit — give a ref, a commit, or a date like 2026-08-01", want)
	}
	return strings.TrimSpace(sha), nil
}

// SinceDiff derives the board at two commits and reports the difference.
// Both ends are commits on purpose: a diff that depended on uncommitted
// working-tree edits would give two people different answers about the same
// repository, which is the failure this tool exists to prevent.
func SinceDiff(repo, from string) (*Diff, error) {
	branches, err := collectBranches(repo)
	if err != nil {
		return nil, err
	}
	elected, _, _, err := electIntegration(repo, branches)
	if err != nil {
		return nil, err
	}
	base := integrationRef(repo, elected)

	fromSHA, err := SinceRef(repo, base, from)
	if err != nil {
		return nil, err
	}
	toSHA, ok := gitrepo.Try(repo, "rev-parse", base)
	if !ok {
		return nil, fmt.Errorf("cannot resolve %s", base)
	}
	toSHA = strings.TrimSpace(toSHA)

	before, err := snapshotAt(repo, fromSHA)
	if err != nil {
		return nil, err
	}
	after, err := snapshotAt(repo, toSHA)
	if err != nil {
		return nil, err
	}
	return compare(from, base, before, after), nil
}

// snapshotAt rebuilds the board as of one commit: the intent files that
// existed in that tree, and the deliveries reachable from it.
func snapshotAt(repo, sha string) (*snapshot, error) {
	s := &snapshot{
		sha:    sha,
		titles: map[string]string{},
		done:   map[string]int{},
		total:  map[string]int{},
		landed: map[string]bool{},
	}
	if when, ok := gitrepo.Try(repo, "show", "-s", "--format=%ct", sha); ok {
		s.date = commitTime(when)
	}

	files, ok := gitrepo.Try(repo, "ls-tree", "-r", "--name-only", sha, "--", spec.RelDir)
	if ok {
		for _, f := range strings.Split(files, "\n") {
			f = strings.TrimSpace(f)
			if !strings.HasSuffix(f, ".md") {
				continue
			}
			body, ok := gitrepo.Try(repo, "show", sha+":"+f)
			if !ok {
				continue
			}
			sp, err := spec.Parse(f, []byte(body))
			if err != nil || sp.ID == "" {
				continue // unparseable then is not a story now
			}
			s.titles[sp.ID] = sp.Title
			s.done[sp.ID], s.total[sp.ID] = spec.Progress(sp.Body)
		}
	}

	known := make(map[string]bool, len(s.titles))
	for id := range s.titles {
		known[id] = true
	}
	walkTrailerCommits(repo, sha, known, func(c trailerCommit) {
		if !c.filing {
			s.landed[c.id] = true
		}
	})
	return s, nil
}

// compare reads the difference between two snapshots. Every branch here is
// a fact one of the two commits can prove; nothing is inferred from the gap.
func compare(fromLabel, toLabel string, before, after *snapshot) *Diff {
	d := &Diff{
		From: fromLabel, FromSHA: short(before.sha), FromDate: before.date.Format(spec.DateLayout),
		To: toLabel, ToSHA: short(after.sha), ToDate: after.date.Format(spec.DateLayout),
	}
	title := func(id string) string {
		if t := after.titles[id]; t != "" {
			return t
		}
		return before.titles[id]
	}

	for _, id := range after.ids() {
		ch := Change{ID: id, Title: title(id)}
		if _, existed := before.titles[id]; !existed {
			d.Filed = append(d.Filed, ch)
		}
		switch {
		case after.landed[id] && !before.landed[id]:
			d.Landed = append(d.Landed, ch)
		case before.landed[id] && !after.landed[id]:
			d.Unlanded = append(d.Unlanded, ch)
		}

		// Ticks are compared only where there is a checklist to compare: a
		// story that gained its acceptance criteria since is filed news, not
		// sign-off news.
		if after.total[id] > 0 && before.total[id] > 0 {
			switch {
			case after.done[id] > before.done[id]:
				d.Ticked = append(d.Ticked, Change{ID: id, Title: ch.Title,
					Detail: fmt.Sprintf("%d/%d → %d/%d criteria", before.done[id], before.total[id], after.done[id], after.total[id])})
			case after.done[id] < before.done[id]:
				d.Unticked = append(d.Unticked, Change{ID: id, Title: ch.Title,
					Detail: fmt.Sprintf("%d/%d → %d/%d criteria — a promise withdrawn", before.done[id], before.total[id], after.done[id], after.total[id])})
			}
		}

		// The one drift a commit proves at both ends: landed work whose
		// promises nobody read back. Newly landed and unticked is drift
		// opening; landed before and fully ticked since is drift closing.
		unverifiedNow := after.landed[id] && after.total[id] > 0 && after.done[id] < after.total[id]
		unverifiedThen := before.landed[id] && before.total[id] > 0 && before.done[id] < before.total[id]
		switch {
		case unverifiedNow && !unverifiedThen:
			d.Unverified = append(d.Unverified, Change{ID: id, Title: ch.Title,
				Detail: fmt.Sprintf("landed with %d of %d criteria ticked", after.done[id], after.total[id])})
		case unverifiedThen && !unverifiedNow && after.landed[id]:
			d.Verified = append(d.Verified, Change{ID: id, Title: ch.Title,
				Detail: fmt.Sprintf("signed off — %d of %d", after.done[id], after.total[id])})
		}
	}

	for _, id := range before.ids() {
		if _, still := after.titles[id]; !still {
			d.Retired = append(d.Retired, Change{ID: id, Title: before.titles[id]})
		}
	}
	return d
}
