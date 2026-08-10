package workspace

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// Candidate is a repository sitting beside the hub that could be declared as
// a spoke. It is a proposal and nothing more: discovery reads, the adopter
// decides. A workspace folder holds plenty that is no spoke — a media
// directory, a pitch deck, somebody's unrelated checkout — and a repo
// declared without being seen is a board gathering proof from something
// nobody meant to watch.
type Candidate struct {
	Name   string // derived from the directory name
	Path   string // relative to the hub, in slash form (../api)
	Remote string // origin, or empty when this repo has none we can read
}

// Discover lists the git repositories next to the hub that its manifest does
// not already declare.
//
// The remotes it reports are read from each checkout's own config, which is
// where they already are: declaring a workspace by hand means transcribing
// them, and transcription is the one cost of adoption that grows with the
// number of repos. Reading a config file is not cloning — discovery never
// fetches, exactly like the audit and adoption around it.
//
// Ordering is by name so a proposal reads the same twice.
func Discover(hub string, declared *Workspace) ([]Candidate, error) {
	hubAbs, err := filepath.Abs(hub)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Dir(hubAbs))
	if err != nil {
		return nil, err
	}

	taken := map[string]bool{}
	claimed := map[string]bool{}
	if declared != nil {
		for _, r := range declared.Repos {
			taken[r.Name] = true
			if r.Path == "" {
				continue
			}
			if abs, err := filepath.Abs(filepath.Join(hubAbs, filepath.FromSlash(r.Path))); err == nil {
				claimed[abs] = true
			}
		}
	}

	var found []Candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(filepath.Dir(hubAbs), e.Name())
		if dir == hubAbs || claimed[dir] {
			continue
		}
		// A directory is offered only if git itself calls it a work tree:
		// a stray .git file, a bare mirror, or a plain folder are all "no".
		if out, ok := gitrepo.Try(dir, "rev-parse", "--is-inside-work-tree"); !ok || out != "true" {
			continue
		}
		name := repoName(e.Name())
		if name == "" || taken[name] || ValidName(name) != nil {
			// A name we cannot safely derive is not a reason to guess one:
			// the adopter can declare that repo by hand with any name.
			continue
		}
		taken[name] = true
		// A checkout with no readable origin is still worth offering as a
		// path-only spoke — the board can read it locally, and skipping it
		// in silence is how a repo ends up watched by nobody.
		remote, _ := gitrepo.Try(dir, "remote", "get-url", "origin")
		rel, err := filepath.Rel(hubAbs, dir)
		if err != nil {
			continue
		}
		found = append(found, Candidate{Name: name, Path: filepath.ToSlash(rel), Remote: remote})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found, nil
}

// Repos renders candidates as declarations ready for Declare.
func Repos(cands []Candidate) []Repo {
	repos := make([]Repo, len(cands))
	for i, c := range cands {
		repos[i] = Repo{Name: c.Name, Remote: c.Remote, Path: c.Path}
	}
	return repos
}

// repoName derives a spoke name from a directory name, lowercasing it and
// folding anything a name may not carry into "-". Names label branches as
// "name:branch", so the rules are ValidName's; a directory that cannot yield
// one is left for the adopter to name.
func repoName(dir string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(dir) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}
