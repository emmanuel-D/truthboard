// Package adopt wires a repository so AI agents route their work through
// Truthboard by default: MCP registration, a standing working agreement,
// and an optional commit-msg nudge. Everything it writes is idempotent —
// marker-delimited blocks are replaced, never duplicated, and files owned
// by others (existing .mcp.json servers, existing hooks) are preserved.
package adopt

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	beginMark = "<!-- truthboard:begin -->"
	endMark   = "<!-- truthboard:end -->"
	hookMark  = "# truthboard trailer nudge"
)

const agreement = beginMark + `
## Truthboard working agreement

This repo tracks work with Truthboard. One rule: **statuses are derived
from git, never typed** — there is no way to set one, so don't look for it.

Before any coding task:

1. Check the board first: ` + "`get_board`" + ` (MCP) or ` + "`truthboard audit`" + ` —
   don't duplicate work that is already in flight.
2. Get your task: ` + "`get_brief <id>`" + ` returns the story's goal, acceptance
   criteria, scope, and linking instructions. For new work, ` + "`create_spec`" + `
   with a full goal and acceptance body — never leave the placeholder.
3. Work on a branch containing the spec id (e.g. ` + "`feature/tb-1234-slug`" + `).
4. End **every** commit message with the trailer line: ` + "`Spec: tb-1234`" + `.
   That trailer is how your work appears on the board with zero extra effort.
5. When acceptance criteria are met, merge — the board flips to done by
   itself. Reverts and red CI flip it to regressed by themselves too.

Story *intent* (title, goal, acceptance, epic, priority, scope) is always
editable — ` + "`update_spec`" + ` over MCP, the CLI, or editing
` + "`.truthboard/specs/*.md`" + ` directly. Commit intent changes like code.
` + endMark + "\n"

const claudePointer = beginMark + `
## Task tracking

This repo uses Truthboard — see the "Truthboard working agreement" section
in AGENTS.md. In short: get your task with ` + "`get_brief`" + `, end every commit
with its ` + "`Spec: <id>`" + ` trailer, and never try to set a status (statuses
are derived from git).
` + endMark + "\n"

// hookNudge is exit-code-neutral so it can sit inside someone else's hook
// without ever changing what that hook decides.
const hookNudge = hookMark + ` — warns when a commit has no Spec trailer; NEVER blocks.
tb_msg=$(cat "$1")
case "$tb_msg" in
  Merge*|Revert*|fixup!*|squash!*) : ;;
  *)
    if ! printf '%s\n' "$tb_msg" | grep -qE '^Spec: tb-'; then
      echo "truthboard: no 'Spec: <id>' trailer — this commit will show up as shadow work (truthboard audit)" >&2
    fi ;;
esac
`

const hookScript = "#!/bin/sh\n" + hookNudge + "exit 0\n"

// Agents performs the wiring and returns a human-readable action log.
func Agents(repo string, hooks bool) ([]string, error) {
	var log []string
	step := func(format string, a ...any) { log = append(log, fmt.Sprintf(format, a...)) }

	if err := os.MkdirAll(filepath.Join(repo, ".truthboard", "specs"), 0o755); err != nil {
		return nil, err
	}

	changed, err := registerMCP(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		return nil, err
	}
	if changed {
		step(".mcp.json: registered the truthboard MCP server")
	} else {
		step(".mcp.json: truthboard already registered")
	}

	agentsPath := filepath.Join(repo, "AGENTS.md")
	if changed, err = upsertBlock(agentsPath, agreement, true); err != nil {
		return nil, err
	}
	step("AGENTS.md: working agreement %s", writtenWord(changed))

	claudePath := filepath.Join(repo, "CLAUDE.md")
	if _, statErr := os.Stat(claudePath); statErr == nil {
		if changed, err = upsertBlock(claudePath, claudePointer, false); err != nil {
			return nil, err
		}
		step("CLAUDE.md: pointer %s", writtenWord(changed))
	} else {
		step("CLAUDE.md: not present, skipped (AGENTS.md carries the agreement)")
	}

	if hooks {
		msg, err := installHook(repo)
		if err != nil {
			return nil, err
		}
		step("commit-msg hook: %s", msg)
	}
	return log, nil
}

func writtenWord(changed bool) string {
	if changed {
		return "written"
	}
	return "already up to date"
}

// registerMCP adds the truthboard server to .mcp.json, preserving any
// other servers and unknown top-level keys.
func registerMCP(path string) (changed bool, err error) {
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("%s exists but is not valid JSON: %w", path, err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, ok := servers["truthboard"]; ok {
		return false, nil
	}
	servers["truthboard"] = map[string]any{"command": "truthboard", "args": []string{"mcp"}}
	doc["mcpServers"] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(path, append(out, '\n'), 0o644)
}

// upsertBlock replaces the marker-delimited block in the file, appends it
// when absent, and (when createIfMissing) creates the file around it.
// Returns false when the file already contains exactly this block.
func upsertBlock(path, block string, createIfMissing bool) (changed bool, err error) {
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		if !os.IsNotExist(readErr) {
			return false, readErr
		}
		if !createIfMissing {
			return false, nil
		}
		return true, os.WriteFile(path, []byte(block), 0o644)
	}
	content := string(raw)
	begin, end := strings.Index(content, beginMark), strings.Index(content, endMark)
	if begin >= 0 && end > begin {
		existing := content[begin : end+len(endMark)+1]
		if existing == block || existing == strings.TrimSuffix(block, "\n") {
			return false, nil
		}
		updated := content[:begin] + block + content[end+len(endMark)+1:]
		return true, os.WriteFile(path, []byte(updated), 0o644)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return true, os.WriteFile(path, []byte(content+"\n"+block), 0o644)
}

// installHook writes the commit-msg nudge, appending to an existing hook
// rather than clobbering it, and never installing twice.
func installHook(repo string) (string, error) {
	hooksDir := filepath.Join(repo, ".git", "hooks")
	if _, err := os.Stat(hooksDir); err != nil {
		return "", fmt.Errorf("no .git/hooks directory — is %s a git repository?", repo)
	}
	path := filepath.Join(hooksDir, "commit-msg")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "installed (warns on missing trailer, never blocks)", os.WriteFile(path, []byte(hookScript), 0o755)
	}
	if err != nil {
		return "", err
	}
	if strings.Contains(string(raw), hookMark) {
		return "already installed", nil
	}
	// Existing hook belongs to someone else: insert the exit-code-neutral
	// nudge right after its shebang so the hook's own logic still decides
	// the outcome — we only ever print.
	content := string(raw)
	if nl := strings.Index(content, "\n"); strings.HasPrefix(content, "#!") && nl > 0 {
		content = content[:nl+1] + "\n" + hookNudge + "\n" + content[nl+1:]
	} else {
		content = hookNudge + "\n" + content
	}
	return "inserted into your existing commit-msg hook (warn-only)", os.WriteFile(path, []byte(content), 0o755)
}
