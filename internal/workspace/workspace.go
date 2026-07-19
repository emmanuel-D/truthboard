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
