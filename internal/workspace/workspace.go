// Package workspace reads the multi-repo manifest: intent lives in one hub
// repo (the one carrying .truthboard/), and proof is gathered from the spoke
// repos declared in .truthboard/workspace.yml. The manifest is itself intent —
// versioned, diffable, edited like any spec. No manifest means a workspace of
// one: single-repo behavior, untouched.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"gopkg.in/yaml.v3"
)

// File is the manifest path relative to the hub root.
const File = ".truthboard/workspace.yml"

// Repo is one declared spoke.
type Repo struct {
	Name        string `yaml:"-"`
	Remote      string `yaml:"remote"`
	Integration string `yaml:"integration"` // optional; empty means elect by activity
	Path        string `yaml:"path"`        // optional local checkout, relative to the hub root
}

// Workspace is the parsed manifest.
type Workspace struct {
	Hub   string // hub repo path as given to Load
	Repos []Repo // sorted by name for deterministic output
}

// namePattern keeps spoke names safe as prefixes: they appear as "name:" in
// branch labels, evidence, and scope paths, so no colons, slashes, or globs.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// ValidName explains why name cannot label a spoke, or nil. One rule for
// the loader and every writer (scaffold, future editors) — a name that
// scaffolds must be a name that loads.
func ValidName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("repo name %q — names label branches as \"name:branch\", so lowercase letters, digits, . _ - only", name)
	}
	if name == "hub" {
		return fmt.Errorf("%q is reserved — it names the repo carrying .truthboard/ in a spec's repos: list", name)
	}
	return nil
}

// Load parses the hub's workspace manifest. A missing manifest returns
// (nil, nil) — that is the single-repo case, not an error. A malformed one
// fails loudly: a silently ignored manifest would be a board lying about
// which repos it watches.
func Load(hub string) (*Workspace, error) {
	raw, err := os.ReadFile(filepath.Join(hub, filepath.FromSlash(File)))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Repos map[string]Repo `yaml:"repos"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", File, err)
	}
	if len(doc.Repos) == 0 {
		return nil, fmt.Errorf("%s: no repos declared — delete the file for single-repo mode", File)
	}
	ws := &Workspace{Hub: hub}
	for name, r := range doc.Repos {
		r.Name = name
		if err := ValidName(name); err != nil {
			return nil, fmt.Errorf("%s: %w", File, err)
		}
		if r.Remote == "" && r.Path == "" {
			return nil, fmt.Errorf("%s: repo %q needs a remote (for the board server to clone) or a path (a local checkout)", File, name)
		}
		ws.Repos = append(ws.Repos, r)
	}
	sort.Slice(ws.Repos, func(i, j int) bool { return ws.Repos[i].Name < ws.Repos[j].Name })
	return ws, nil
}

// CloneDir is where the board server keeps its managed mirror clone of a
// spoke: inside the hub's git dir, next to the lifecycle state — never in
// anyone's working tree.
func CloneDir(hub, name string) string {
	gitDir, ok := gitrepo.Try(hub, "rev-parse", "--git-dir")
	if !ok {
		gitDir = ".git"
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(hub, gitDir)
	}
	return filepath.Join(gitDir, "truthboard", "spokes", name)
}

// Resolve returns the local path proof is read from, strictly read-only:
// the declared checkout if it exists, else the server's managed clone if it
// exists. When neither does, the error says how to get one — the audit
// reports it and moves on; it never clones (the audit engine must not touch
// the network or disk on anyone's behalf).
func (w *Workspace) Resolve(r Repo) (string, error) {
	if r.Path != "" {
		p := r.Path
		if !filepath.IsAbs(p) {
			p = filepath.Join(w.Hub, p)
		}
		if _, err := os.Stat(p); err == nil {
			if err := verifyIdentity(p, r); err != nil {
				return "", err
			}
			return p, nil
		}
		if r.Remote == "" {
			return "", fmt.Errorf("declared path %s does not exist", r.Path)
		}
	}
	clone := CloneDir(w.Hub, r.Name)
	if _, err := os.Stat(clone); err == nil {
		return clone, nil
	}
	return "", fmt.Errorf("no local copy — start the board server (it clones spokes) or set path: in %s", File)
}

// verifyIdentity refuses a declared path holding a checkout of some other
// repository. Existence alone used to be the whole test, so a mistyped or
// stale path made the board read proof from the wrong repo and report it with
// full confidence — the one wrong answer this tool cannot afford, and the only
// silent one. Reading a remote never mutates or fetches, so the read-only
// doctrine is intact.
//
// Only a proven mismatch refuses. A path-only spoke has nothing to compare
// against, and a checkout whose origin cannot be read (no origin, a mirror, a
// differently-named remote) proves nothing either — refusing those would cry
// wolf on valid setups, and a check people switch off protects nobody.
func verifyIdentity(path string, r Repo) error {
	if r.Remote == "" {
		return nil
	}
	origin, ok := gitrepo.Try(path, "remote", "get-url", "origin")
	if !ok || origin == "" {
		return nil
	}
	if sameRemote(origin, r.Remote) {
		return nil
	}
	return fmt.Errorf("declared path %s is a checkout of %s, not %s — fix path: or remote: in %s",
		r.Path, origin, r.Remote, File)
}

// sameRemote reports whether two remote URLs name the same repository across
// the forms that mean the same thing: scp-style against https, an optional
// .git suffix or trailing slash, and embedded credentials.
func sameRemote(a, b string) bool {
	return normalizeRemote(a) == normalizeRemote(b)
}

// normalizeRemote reduces a remote URL to host/owner/repo.
func normalizeRemote(raw string) string {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	} else if at := strings.Index(s, "@"); at >= 0 && strings.Contains(s[at:], ":") {
		// scp-style, git@host:owner/repo.git — the colon is a separator,
		// not a port, so it becomes the path slash.
		s = strings.Replace(s[at+1:], ":", "/", 1)
	}
	// Credentials survive the scheme strip in URL forms: user:token@host/…
	if at := strings.Index(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git"), "/")
	return strings.ToLower(s)
}
