package adopt

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// jetbrains is the one MCP config no truthboard command writes: Copilot in a
// JetBrains IDE reads neither `.mcp.json` nor `.vscode/mcp.json`, but a
// per-user, per-machine file shared by every project the IDE opens.
//
// Warn, never write. Everything adoption writes lands inside the repository it
// was pointed at — reviewable in the adoption diff, revertible with git,
// committed for the team. Reaching into $HOME to change config for projects
// truthboard was never invited into is a different act. This is the doctor,
// following spawnWarning exactly: name the problem, name the file, print the
// fix, exit successfully.
type jetbrains struct {
	config  string // absolute path to the config, read to see if it is wired
	display string // how the log spells it, with $HOME abbreviated to ~
}

// jetbrainsConfig locates that file, and reports false when this platform is
// one we cannot advise on. Windows keeps the JetBrains Copilot config
// somewhere other than ~/.config, and spawnWarning's precedent is to skip a
// platform rather than print a Unix path to a user who has no such path: a
// confidently wrong location is worse than silence, because it sends someone
// to create a file the IDE will never read.
func jetbrainsConfig() (jetbrains, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return jetbrains{}, false
	}
	return jetbrainsConfigFor(runtime.GOOS, home)
}

// jetbrainsConfigFor is that decision as a pure function of the platform and
// the home directory, so the platform we stay silent on is a tested promise
// rather than a build tag nobody's CI exercises.
func jetbrainsConfigFor(goos, home string) (jetbrains, bool) {
	if goos == "windows" || home == "" {
		return jetbrains{}, false
	}
	rel := ".config/github-copilot/intellij/mcp.json"
	return jetbrains{config: filepath.Join(home, filepath.FromSlash(rel)), display: "~/" + rel}, true
}

// warning is the adoption log's line about a repo opened in IntelliJ whose IDE
// agent has no board. Pure over the repo path and the config location, so the
// tests never touch a real $HOME.
//
// hub is the repository the MCP server must serve, absolute: this file is not
// committed, so the no-machine-local-paths rule that governs `.mcp.json` does
// not govern it, and a global slot shared by every project cannot rely on the
// working directory the IDE happens to spawn a stdio server in.
func (j jetbrains) warning(repo, hub string) []string {
	if !hasIdea(repo) || registersTruthboard(j.config) {
		return nil
	}
	return []string{
		"⚠ this repo carries a .idea/ directory, and Copilot in a JetBrains IDE reads",
		"  neither MCP file just written: its own config is per-machine, at",
		fmt.Sprintf("  %s — the one config no truthboard", j.display),
		"  command writes, so until it carries this, agents in that IDE work with no",
		"  board. Fix — paste it there (Copilot status bar → Edit Settings → Model",
		"  Context Protocol → Configure):",
		fmt.Sprintf(`    {"servers": {"truthboard": {"type": "stdio", "command": "truthboard", "args": ["mcp", %q]}}}`, hub),
	}
}

// ideaSearchDepth bounds how far below the repo root a `.idea/` counts. A
// JetBrains project root is the repo itself or a module a level or two down;
// searching deeper would buy nothing and make adoption pay for the size of the
// tree it was pointed at.
const ideaSearchDepth = 3

// hasIdea reports whether the repository is opened in a JetBrains IDE, as far
// as the disk can say.
//
// The filesystem, never git: `.idea/` is very often gitignored, and an ignored
// `.idea/` is still a developer using IntelliJ — asking git would stay silent
// for exactly the repos most likely to need the warning.
//
// It is an imperfect signal in the other direction too, which is the argument
// for warning rather than acting: `.idea/` says *IntelliJ*, not *which
// assistant* — Copilot, JetBrains AI Assistant and Junie each keep their own
// MCP config.
func hasIdea(repo string) bool {
	found := false
	// Errors are swallowed by the callback, so the walk itself cannot fail;
	// an unreadable directory is one place a .idea/ might have been, not a
	// reason to fail a wiring that has otherwise succeeded.
	_ = filepath.WalkDir(repo, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == repo {
			return nil
		}
		if d.Name() == ".idea" {
			found = true
			return filepath.SkipAll
		}
		// No .idea/ is ever nested inside a dotted directory, and .git is the
		// one every repo has; node_modules and vendor are the two that cost
		// real time. Skipping them keeps this a cheap check on any repo.
		if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" || d.Name() == "vendor" {
			return fs.SkipDir
		}
		if depthBelow(repo, path) >= ideaSearchDepth {
			return fs.SkipDir
		}
		return nil
	})
	return found
}

// depthBelow counts path's directory levels below root: 1 for a child.
func depthBelow(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ideaSearchDepth
	}
	return len(strings.Split(filepath.ToSlash(rel), "/"))
}

// registersTruthboard reports whether the config already names a truthboard
// server, so a wired machine is not nagged on every re-run — the same courtesy
// spawnWarning extends.
//
// Absent, unreadable and malformed all answer false: the warning fires and
// adoption still succeeds. Reading a file we cannot parse as "already wired"
// would silently withhold the one thing this step exists to say, and nothing
// here ever rewrites that file, so an unparseable one is only ever read.
//
// Both spellings count. JetBrains Copilot uses VS Code's schema, so `servers`
// is what a paste of the snippet above produces — but a hand-written
// `mcpServers` entry is someone who has already done the wiring, and telling
// them to do it again is the nagging this check exists to prevent.
func registersTruthboard(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var doc map[string]map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		return false
	}
	for _, key := range []string{vscodeMCPKey, "mcpServers"} {
		if _, ok := doc[key]["truthboard"]; ok {
			return true
		}
	}
	return false
}
