package adopt

import (
	"fmt"
	"path"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// writtenFiles are the agent-facing files adoption writes, in the order it
// writes them. Every one is meant to be committed — that is the whole point,
// since a wiring only a laptop has is a wiring the team does not.
var writtenFiles = []string{".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md"}

// ignoreRule is git's own account of why a path will not be kept: the file
// that excludes it, the line, and the pattern doing the work.
type ignoreRule struct {
	source  string
	line    string
	pattern string
}

// ignoredBy asks git whether a written file is excluded from this repository.
//
// Adoption used to report such a write as a plain success: wiring lettalk-server
// logged ".vscode/mcp.json: registered" while that repo's .gitignore excluded
// .vscode/* — true about the write, misleading about the outcome, and invisible
// to the audit because the file is right there on the machine auditing it.
//
// A tracked file is never ignored, whatever the rules say, so ls-files decides
// first: warning about a committed .mcp.json would teach people to skim past the
// warning that matters. A directory that is not a git repository answers neither
// question and is left alone.
func ignoredBy(repo, rel string) (ignoreRule, bool) {
	if _, tracked := gitrepo.Try(repo, "ls-files", "--error-unmatch", "--", rel); tracked {
		return ignoreRule{}, false
	}
	out, ok := gitrepo.Try(repo, "check-ignore", "-v", "--", rel)
	if !ok || out == "" {
		return ignoreRule{}, false
	}
	// `<source>:<line>:<pattern>\t<path>` — the path is echoed back after a
	// tab, so the rule is everything before it.
	rule := out
	if tab := strings.IndexByte(rule, '\t'); tab >= 0 {
		rule = rule[:tab]
	}
	parts := strings.SplitN(rule, ":", 3)
	if len(parts) != 3 {
		return ignoreRule{}, false
	}
	// check-ignore exits 0 for a *negated* match too, reporting the "!" rule
	// that re-includes the path. Reading the exit code alone would warn about
	// every repo that already carries the exception — precisely the repos that
	// took the advice.
	if strings.HasPrefix(parts[2], "!") {
		return ignoreRule{}, false
	}
	return ignoreRule{source: parts[0], line: parts[1], pattern: parts[2]}, true
}

// exception renders .gitignore lines that actually re-include rel.
//
// Git cannot re-include a file whose parent directory is excluded — it never
// descends into it, so a lone "!.vscode/mcp.json" under a ".vscode/" rule is a
// line that looks like a fix and does nothing. Each excluded ancestor therefore
// has to be re-included and its contents re-excluded before the file itself can
// be named. Which ancestors those are is git's answer, not a guess from the
// pattern's shape: check-ignore is asked about every one.
func exception(repo, rel string) []string {
	var lines []string
	parts := strings.Split(path.Dir(rel), "/")
	if path.Dir(rel) != "." {
		for i := range parts {
			ancestor := path.Join(parts[:i+1]...)
			// Probed as a plain path, with no trailing slash: git reads the
			// filesystem to see it is a directory, and every rule shape then
			// answers correctly — `.vscode/` and `.vscode` match, `.vscode/*`
			// does not, which is exactly the distinction that decides whether a
			// lone negation can work. Spelling it "ancestor/" instead would
			// make `.vscode/*` match the trailing slash as the empty string and
			// report the parent excluded when it is not. Sound because the
			// directory exists by the time anything asks: adoption has just
			// written into it, and drift reads a checkout that already has it.
			if _, ignored := gitrepo.Try(repo, "check-ignore", "-q", "--", ancestor); !ignored {
				continue
			}
			lines = append(lines, "!"+ancestor+"/", ancestor+"/*")
		}
	}
	return append(lines, "!"+rel)
}

// ignoreWarning is the adoption log's line about a file this repo will throw
// away. Warn-only, like spawnWarning and RepoWarning: the file on disk is
// correct and adoption's job is to write it and say what it knows, never to
// edit someone's .gitignore on their behalf.
func ignoreWarning(repo, rel string) []string {
	rule, ignored := ignoredBy(repo, rel)
	if !ignored {
		return nil
	}
	fix := exception(repo, rel)
	quoted := make([]string, len(fix))
	for i, line := range fix {
		quoted[i] = "'" + line + "'"
	}
	return []string{
		fmt.Sprintf("⚠ %s is written, but %s:%s (%s) excludes it — it works here and", rel, rule.source, rule.line, rule.pattern),
		"  cannot be committed, so every teammate and every fresh clone gets this repo",
		"  with no board. Fix:",
		fmt.Sprintf("    printf '%%s\\n' %s >> %s", strings.Join(quoted, " "), rule.source),
	}
}

// ignoredWiring reports wiring that is present in a spoke and excluded from it.
//
// The same failure the unwired-spoke finding exists for, one step further out:
// there the agent working the repo has no board, here the agent working a
// *clone* of it has none, and nothing on the auditing machine looks wrong. Its
// own repo, its own rules — the hub's .gitignore says nothing about a spoke's.
func ignoredWiring(checkout string) []string {
	var found []string
	for _, rel := range writtenFiles {
		rule, ignored := ignoredBy(checkout, rel)
		if !ignored {
			continue
		}
		found = append(found, fmt.Sprintf(
			"%s is wired here but excluded by %s:%s (%s) — a fresh clone of this spoke has no board; add %s to %s",
			rel, rule.source, rule.line, rule.pattern, strings.Join(exception(checkout, rel), " + "), rule.source))
	}
	return found
}
