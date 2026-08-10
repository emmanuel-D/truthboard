package adopt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// Gaps reports declared spokes whose checkout is on disk but is not wired
// for agents. Wiring a spoke once is not the same as a spoke staying wired:
// a fresh clone, a hand-edited .mcp.json, or a spoke declared after the last
// setup run all leave a repo the board watches for proof whose agents have
// no board — silent until trailerless commits surface as shadow work.
//
// Read-only by construction: it states what is missing and how to fix it,
// and never writes. Living here rather than in the audit keeps one owner for
// what "wired" means — the code that writes these files decides when they
// count as present.
//
// A spoke with no local checkout is never a gap. The audit already reports
// an unreadable spoke by name, and saying the same thing twice in two
// vocabularies teaches people to skim both.
func Gaps(hub string) []string {
	ws, err := workspace.Load(hub)
	if err != nil || ws == nil {
		return nil
	}
	hubHasNudge := hasNudge(hub)

	var gaps []string
	for _, r := range ws.Repos {
		checkout, err := ws.Checkout(r)
		if err != nil {
			continue
		}
		// Wiring that is present but excluded gets its own finding: re-running
		// adoption rewrites a file the repo already has and still throws away,
		// so pooling it with the missing-wiring gaps would name the one fix
		// that cannot work.
		for _, line := range ignoredWiring(checkout) {
			gaps = append(gaps, fmt.Sprintf("%s (%s): %s", r.Name, r.Path, line))
		}
		missing := spokeGaps(checkout, hub)
		if len(missing) == 0 {
			continue
		}
		// The nudge is opt-in (--hooks), so its absence is never a finding
		// of its own — a warning about a feature nobody asked for is one
		// people learn to ignore. Named only when the hub has one and this
		// spoke is already being reported: then it is part of what the
		// re-run will fix.
		if hubHasNudge && !hasNudge(checkout) {
			missing = append(missing, "no commit-msg trailer nudge (the hub has one)")
		}
		gaps = append(gaps, fmt.Sprintf("%s (%s): %s — re-run `truthboard init --workspace` in the hub",
			r.Name, r.Path, strings.Join(missing, ", ")))
	}
	return gaps
}

// spokeGaps lists what a checked-out spoke is missing, in the order an agent
// would hit it: no server to ask, a server pointed at the wrong repository,
// then nothing telling it the agreement.
func spokeGaps(checkout, hub string) []string {
	var missing []string
	served, ok := servedRepo(filepath.Join(checkout, ".mcp.json"), "mcpServers")
	switch {
	case !ok:
		missing = append(missing, "no MCP registration (agents here have no board)")
	case served == "":
		missing = append(missing, "MCP server serves this repo, which has no specs — it must serve the hub")
	case !sameDir(filepath.Join(checkout, served), hub):
		missing = append(missing, fmt.Sprintf("MCP server points at %s, not the hub", served))
	}
	if !hasAgreement(filepath.Join(checkout, "AGENTS.md")) {
		missing = append(missing, "no working agreement in AGENTS.md")
	}
	return missing
}

// servedRepo returns the repository argument of the registered truthboard
// server: ok is false when no such server is registered, and an empty path
// means it was registered without one (correct in a hub, wrong in a spoke).
func servedRepo(path, key string) (served string, ok bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var doc map[string]map[string]struct {
		Args []string `json:"args"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		// Unparseable is not "unregistered": adoption itself refuses to
		// touch such a file, so reporting it as missing wiring would send
		// people to a command that cannot fix it.
		return "", true
	}
	server, exists := doc[key]["truthboard"]
	if !exists {
		return "", false
	}
	for _, a := range server.Args {
		if a != "mcp" && !strings.HasPrefix(a, "-") {
			return a, true
		}
	}
	return "", true
}

// hasAgreement reports whether the file carries a truthboard-written block.
func hasAgreement(path string) bool {
	raw, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(raw), beginMark)
}

// hasNudge reports whether a repository's commit-msg hook carries the nudge.
func hasNudge(repo string) bool {
	raw, err := os.ReadFile(filepath.Join(repo, ".git", "hooks", "commit-msg"))
	return err == nil && strings.Contains(string(raw), hookMark)
}

// sameDir compares two paths as directories, resolving symlinks so a spoke
// wired through /var and a hub read through /private/var still match.
func sameDir(a, b string) bool {
	clean := func(p string) string {
		abs, err := filepath.Abs(p)
		if err != nil {
			return filepath.Clean(p)
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			return resolved
		}
		return abs
	}
	return clean(a) == clean(b)
}
