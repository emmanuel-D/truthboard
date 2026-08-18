package adopt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// Uninstall takes the wiring back out of a repository: the inverse of wire().
// It writes nothing unless apply is true — the plan is the default, the way
// `mirror` shows its plan before it publishes, because the surgery here runs
// through files the adopter owns.
//
// Three of the six things adoption writes are merged into someone else's
// file: the marker block inside a CLAUDE.md they tuned by hand, the nudge
// inside a commit-msg hook that has its own logic, and one server among
// others in .mcp.json. Those are exactly the removals a person doing this by
// hand gets wrong, and the two under .git/ are ones they never see at all —
// a hook that outlives the binary warns about a tool that is no longer
// installed, on every commit.
//
// Specs are not wiring. `.truthboard/` is the adopter's intent and their
// history, so it survives unless specs is true and they asked for it.
func Uninstall(repo string, apply, specs bool) ([]string, error) {
	var log []string
	step := func(format string, a ...any) { log = append(log, fmt.Sprintf(format, a...)) }

	for _, f := range []struct {
		path, key string
	}{
		{".mcp.json", "mcpServers"},
		{".vscode/mcp.json", vscodeMCPKey},
	} {
		msg, err := unregisterMCP(filepath.Join(repo, f.path), f.key, apply)
		if err != nil {
			return nil, err
		}
		step("%s: %s", f.path, msg)
	}

	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		msg, err := removeBlock(filepath.Join(repo, name), apply)
		if err != nil {
			return nil, err
		}
		step("%s: %s", name, msg)
	}

	msg, err := removeNudge(repo, apply)
	if err != nil {
		return nil, err
	}
	step("commit-msg hook: %s", msg)

	npm, err := removeNpmScripts(repo, apply)
	if err != nil {
		return nil, err
	}
	log = append(log, npm...)

	stateMsg, err := removeRunState(repo, apply)
	if err != nil {
		return nil, err
	}
	step("%s", stateMsg)

	// Said last, and said either way: the one thing this command will not do
	// on its own is the one thing that cannot be undone.
	specDir := filepath.Join(repo, ".truthboard")
	switch _, statErr := os.Stat(specDir); {
	case statErr != nil:
		// nothing there to speak about
	case !specs:
		step(".truthboard/: kept — your stories live here, and git still carries")
		step("  their history. Delete them with --specs, or by hand when you mean it")
	case !apply:
		step(".truthboard/: would delete every story, as --specs asks")
	default:
		if err := os.RemoveAll(specDir); err != nil {
			return nil, err
		}
		step(".truthboard/: deleted every story, as --specs asks")
	}
	return log, nil
}

// verb renders an action in the tense the run is actually in, so a plan can
// never be misread as a report of something already done.
func verb(apply bool, did, would string) string {
	if apply {
		return did
	}
	return "would " + would
}

// unregisterMCP removes the truthboard server from an MCP config, leaving
// every other server and unknown key untouched, and deleting the file only
// when truthboard's entry was the whole of it. A file that is not valid JSON
// is an error here exactly as it is in registerMCP: it is someone's config,
// and guessing at its contents is worse than stopping.
func unregisterMCP(path, key string, apply bool) (string, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "not here", nil
	}
	if err != nil {
		return "", err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("%s exists but is not valid JSON: %w", path, err)
	}
	servers, _ := doc[key].(map[string]any)
	if _, ok := servers["truthboard"]; !ok {
		return "no truthboard server registered", nil
	}
	delete(servers, "truthboard")
	others := len(servers)
	if others == 0 {
		delete(doc, key)
	} else {
		doc[key] = servers
	}

	if len(doc) == 0 {
		if apply {
			if err := os.Remove(path); err != nil {
				return "", err
			}
		}
		return verb(apply, "removed — it held nothing but the truthboard server",
			"be removed — it holds nothing but the truthboard server"), nil
	}
	if apply {
		out, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
			return "", err
		}
	}
	kept := ""
	if others > 0 {
		kept = fmt.Sprintf(", keeping your %d other server(s)", others)
	}
	return verb(apply, "truthboard server removed"+kept,
		"remove the truthboard server"+kept), nil
}

// removeBlock cuts the marker-delimited block out of a file the adopter
// owns, leaving every line they wrote byte-identical. A file that was
// nothing but the block is deleted: adoption created it, so uninstall is
// what un-creates it.
func removeBlock(path string, apply bool) (string, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "not here", nil
	}
	if err != nil {
		return "", err
	}
	content := string(raw)
	begin, end := strings.Index(content, beginMark), strings.Index(content, endMark)
	if begin < 0 || end <= begin {
		return "no truthboard block — left alone", nil
	}
	stop := end + len(endMark)
	if stop < len(content) && content[stop] == '\n' {
		stop++
	}
	rest := content[:begin] + content[stop:]

	if strings.TrimSpace(rest) == "" {
		if apply {
			if err := os.Remove(path); err != nil {
				return "", err
			}
		}
		return verb(apply, "removed — truthboard wrote the whole file",
			"be removed — truthboard wrote the whole file"), nil
	}
	if apply {
		// The block was appended after a blank line; take that separator with
		// it so a re-adopt/uninstall cycle cannot creep blank lines into the
		// file. Anything the adopter wrote is untouched.
		if err := os.WriteFile(path, []byte(strings.TrimRight(rest, "\n")+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	return verb(apply, "truthboard block removed, your own content kept",
		"remove the truthboard block, keeping your own content"), nil
}

// removeNudge takes the trailer nudge out of the commit-msg hook. The hook
// may be someone else's — adoption inserted into it deliberately — so only
// the nudge leaves, and only when it is one truthboard can prove it wrote:
// bounded by hookEndMark, or an exact match for a version in legacyNudges.
// Anything else is left alone and said out loud, on the same rule that keeps
// upgradeNudge from rewriting a block it cannot account for.
func removeNudge(repo string, apply bool) (string, error) {
	path := filepath.Join(repo, ".git", "hooks", "commit-msg")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "not here", nil
	}
	if err != nil {
		return "", err
	}
	content := string(raw)
	begin := strings.Index(content, hookMark)
	if begin < 0 {
		return "no truthboard nudge — left alone", nil
	}

	stop := -1
	if end := strings.Index(content[begin:], hookEndMark); end >= 0 {
		stop = begin + end + len(hookEndMark)
	} else {
		for _, old := range legacyNudges {
			if i := strings.Index(content, old); i >= 0 {
				begin, stop = i, i+len(old)
				break
			}
		}
	}
	if stop < 0 {
		return "left alone — the nudge here is not one truthboard wrote, so removing it" +
			" blind could break your hook; delete the block by hand if you want it gone", nil
	}
	if stop < len(content) && content[stop] == '\n' {
		stop++
	}
	// installHook wrapped the nudge in a newline on each side when it inserted
	// into someone else's hook. Take those back too, or a hook that is adopted
	// and abandoned a few times grows a blank line every round trip.
	before, after := content[:begin], content[stop:]
	if strings.HasSuffix(before, "\n\n") && strings.HasPrefix(after, "\n") {
		before, after = before[:len(before)-1], after[1:]
	}
	rest := before + after

	// What hookScript leaves behind once its nudge is cut: a shebang and an
	// exit. Nothing of anyone else's is in it, so the file goes too.
	if isOurEmptyHook(rest) {
		if apply {
			if err := os.Remove(path); err != nil {
				return "", err
			}
		}
		return verb(apply, "removed — truthboard wrote the whole hook",
			"be removed — truthboard wrote the whole hook"), nil
	}
	if apply {
		if err := os.WriteFile(path, []byte(rest), 0o755); err != nil {
			return "", err
		}
	}
	return verb(apply, "nudge removed, your own hook kept",
		"remove the nudge, keeping your own hook"), nil
}

// isOurEmptyHook reports whether what is left of a hook is only the scaffolding
// truthboard itself wrote around the nudge.
func isOurEmptyHook(rest string) bool {
	for _, line := range strings.Split(rest, "\n") {
		switch strings.TrimSpace(line) {
		case "", "#!/bin/sh", "exit 0":
		default:
			return false
		}
	}
	return true
}

// removeNpmScripts deletes the board scripts that are still verbatim ours.
// One the adopter has since edited is theirs now — it is kept, and said so,
// the same way NpmScripts refuses to overwrite one it did not write.
func removeNpmScripts(repo string, apply bool) ([]string, error) {
	if _, err := os.Stat(filepath.Join(repo, "package.json")); err != nil {
		return nil, nil
	}
	if !npmAvailable() {
		return []string{"package.json: npm is not on PATH — board scripts left as they are"}, nil
	}
	var removed, kept []string
	for _, s := range boardScripts {
		name, cmd := s[0], s[1]
		current, exists := npmPkgGet(repo, "scripts."+name)
		switch {
		case !exists:
		case current != cmd:
			kept = append(kept, fmt.Sprintf("%s (yours now: %q)", name, current))
		default:
			if apply {
				if err := npmPkgDelete(repo, "scripts."+name); err != nil {
					return nil, err
				}
			}
			removed = append(removed, name)
		}
	}
	var log []string
	if len(removed) > 0 {
		log = append(log, "package.json: "+verb(apply, "removed scripts ", "remove scripts ")+strings.Join(removed, ", "))
	}
	for _, k := range kept {
		log = append(log, "package.json: keeping your edited script "+k)
	}
	if len(log) == 0 {
		log = append(log, "package.json: no truthboard scripts in it")
	}
	return log, nil
}

// removeRunState clears the board's run directory under .git/. Nothing in it
// is the adopter's: a recorded pid, and the baselines the notifier and
// digester use to decide what is news. It never shows in `git status`, which
// is exactly why leaving it is not an option.
func removeRunState(repo string, apply bool) (string, error) {
	gitDir, ok := gitrepo.Try(repo, "rev-parse", "--absolute-git-dir")
	if !ok {
		return "run state: no git directory here — nothing to clear", nil
	}
	dir := filepath.Join(strings.TrimSpace(gitDir), "truthboard")
	if _, err := os.Stat(dir); err != nil {
		return ".git/truthboard/: not here", nil
	}
	if apply {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	return ".git/truthboard/: " + verb(apply, "run state cleared", "clear the run state (pid, digest and notify baselines)"), nil
}

// Binary is the last word of an uninstall: the half install.sh owns, which a
// command cannot do for itself while it is the thing running.
func Binary(exe string) []string {
	if exe == "" {
		return nil
	}
	return []string{
		"",
		"The binary itself is not this command's to remove — it is running. When you",
		"are done with it:",
		fmt.Sprintf("    rm %s        (or: brew uninstall truthboard)", exe),
	}
}
