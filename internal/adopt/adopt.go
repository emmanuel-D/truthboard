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
	"runtime"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/workspace"
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
   criteria, scope, and linking instructions. No task named? ` + "`next_spec`" + `
   (CLI: ` + "`truthboard next`" + `) hands you the highest-priority planned
   story. For new work, ` + "`create_spec`" + ` with a full goal and acceptance
   body — never leave the placeholder.
3. Work on a branch containing the spec id (e.g. ` + "`feature/tb-1234-slug`" + `).
4. End **every** commit message with the trailer line: ` + "`Spec: tb-1234`" + `.
   That trailer is how your work appears on the board with zero extra effort.
5. When acceptance criteria are met, merge — the board flips to done by
   itself. Reverts and red CI flip it to regressed by themselves too.

Story *intent* (title, goal, acceptance, epic, priority, scope) is always
editable — ` + "`update_spec`" + ` over MCP, the CLI, or editing
` + "`.truthboard/specs/*.md`" + ` directly. Commit intent changes like code.
`

// workspaceGuidance is appended inside the agreement block when the repo
// carries a workspace manifest: decomposition is the practice that keeps
// multi-repo statuses honest, and the agent picking up a fat story is the
// one who should do the splitting — never the PO on a phone.
const workspaceGuidance = `
### Multi-repo workspace

This hub gathers proof from the repos declared in
` + "`.truthboard/workspace.yml`" + `: %s. When a brief's acceptance clearly
spans more than one of them, don't carry one fat story — one provable
landing per spec keeps every status honest:

- **Split it**: narrow this story to a single repo's half
  (` + "`update_spec`" + ` — retitle, adjust acceptance), then ` + "`create_spec`" + ` a
  sibling per remaining repo under the same epic, ordered with ` + "`needs:`" + `.
  Never leave an orphan story that no branch will ever match.
- **Or declare it**: when the story is genuinely one promise that is only
  true once every repo has it, set ` + "`repos: [api, web]`" + ` (` + "`hub`" + ` names
  this repo) — done then requires the trailer landed in every one, with
  per-repo evidence in the meantime.

The id namespace and the ` + "`Spec:`" + ` trailer work identically in every
repo of the workspace.
`

// agreementText renders the working agreement, appending workspace
// guidance when a manifest declares spokes. upsertBlock replaces the whole
// marker block, so adding a workspace later just means re-running adopt.
func agreementText(ws *workspace.Workspace) string {
	if ws == nil || len(ws.Repos) == 0 {
		return agreement + endMark + "\n"
	}
	names := make([]string, len(ws.Repos))
	for i, r := range ws.Repos {
		names[i] = "`" + r.Name + "`"
	}
	return agreement + fmt.Sprintf(workspaceGuidance, strings.Join(names, ", ")) + endMark + "\n"
}

// claudePointer must import AGENTS.md rather than merely mention it:
// Claude Code loads CLAUDE.md into context but never AGENTS.md, so a
// prose reference leaves the agreement unread (proven by a pilot where
// an agent shipped a feature with no spec, branch, or trailer).
const claudePointer = beginMark + `
## Task tracking

This repo uses Truthboard. The working agreement below is imported from
AGENTS.md — follow it: get your task with ` + "`get_brief`" + `, end every commit
with its ` + "`Spec: <id>`" + ` trailer, and never try to set a status (statuses
are derived from git).

@AGENTS.md
` + endMark + "\n"

// hookNudge is exit-code-neutral so it can sit inside someone else's hook
// without ever changing what that hook decides.
const hookNudge = hookMark + ` — warns when a commit has no Spec trailer; NEVER blocks.
tb_msg=$(cat "$1")
# Intent commits (specs, agreement, MCP registration) are exempt from shadow
# work in the audit, so warning about them here would only teach you to
# ignore the hook. Same governed fileset as audit.governedFile.
# Staying silent is the exception: an unreadable or empty staged list falls
# through to the warning, never past it.
tb_state=empty
for tb_f in $(git diff --cached --name-only 2>/dev/null); do
  case "$tb_f" in
    .truthboard/*|.mcp.json|AGENTS.md|CLAUDE.md) tb_state=governed ;;
    *) tb_state=code; break ;;
  esac
done
tb_governed=0
[ "$tb_state" = governed ] && tb_governed=1
case "$tb_msg" in
  Merge*|Revert*|fixup!*|squash!*) : ;;
  *)
    if [ "$tb_governed" = 0 ] && ! printf '%s\n' "$tb_msg" | grep -qE '^Spec: tb-'; then
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
	if runtime.GOOS != "windows" {
		exe, exeErr := os.Executable()
		if exeErr != nil {
			exe = "" // no warning: we cannot name a fix without a location
		} else {
			exe = filepath.Clean(exe)
		}
		for _, line := range spawnWarning(exe, systemPathDirs) {
			step("%s", line)
		}
	}

	// A malformed manifest must not block adoption; the audit already
	// reports it loudly, so the agreement simply omits the guidance.
	ws, _ := workspace.Load(repo)

	agentsPath := filepath.Join(repo, "AGENTS.md")
	if changed, err = upsertBlock(agentsPath, agreementText(ws), true); err != nil {
		return nil, err
	}
	step("AGENTS.md: working agreement %s", writtenWord(changed))
	if ws != nil && len(ws.Repos) > 0 {
		step("AGENTS.md: includes multi-repo decomposition guidance (%d workspace repos)", len(ws.Repos))
	}

	claudePath := filepath.Join(repo, "CLAUDE.md")
	if changed, err = upsertBlock(claudePath, claudePointer, true); err != nil {
		return nil, err
	}
	step("CLAUDE.md: agreement import %s", writtenWord(changed))

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

// RepoWarning tells the adopter when the directory being wired is not a git
// repository. Every truthboard status is derived from branches, merges and
// commit trailers, so the files init writes are inert until one exists — and
// a multi-repo hub, the one repo people create as an empty directory by hand,
// is exactly where that bites. Warn-only like spawnWarning: the wiring is
// still correct on disk, and `git init` is the adopter's call to make.
func RepoWarning(repo string) []string {
	if _, err := gitrepo.Run(repo, "rev-parse", "--is-inside-work-tree"); err == nil {
		return nil
	}
	return []string{
		"⚠ this is not a git repository yet — truthboard derives every status from",
		"  git history, so the board stays empty and `truthboard audit` fails until",
		"  one exists. The specs written here are intent: commit them like code. Fix:",
		"    git init && git add -A && git commit -m \"Track work with truthboard\"",
	}
}

// systemPathDirs are the locations executables stay resolvable from even
// for GUI-launched agents (Claude Code desktop, IDEs) — a login shell's
// PATH additions like ~/go/bin never reach those processes.
var systemPathDirs = []string{"/usr/local/bin", "/opt/homebrew/bin", "/usr/bin", "/bin"}

// spawnWarning tells the adopter when the bare "truthboard" command that
// registerMCP writes cannot be resolved outside their shell profile — the
// failure mode is an agent whose MCP connection dies silently and who then
// works the repo with no board at all. Returns nil when some system dir
// already carries the binary. .mcp.json is committed and shared, so the
// remedy is a symlink on this machine, never an absolute path in the file.
func spawnWarning(exe string, dirs []string) []string {
	for _, dir := range dirs {
		if info, err := os.Stat(filepath.Join(dir, "truthboard")); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return nil
		}
	}
	if exe == "" {
		return nil
	}
	return []string{
		fmt.Sprintf("⚠ agents may not be able to spawn truthboard: none of %s has it,", strings.Join(dirs, ", ")),
		fmt.Sprintf("  and the binary at %s is only on PATH inside your shell profile.", exe),
		"  GUI-launched agents will fail the MCP connection silently. Fix:",
		fmt.Sprintf("    ln -s %s %s/truthboard   (sudo if needed)", exe, dirs[0]),
	}
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
