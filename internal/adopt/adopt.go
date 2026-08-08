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
	// hookEndMark closes the nudge so any later version can find its own
	// block and replace exactly that, leaving the rest of the hook untouched.
	hookEndMark = hookMark + " end"
)

// agreementSteps is the part of the working agreement that is true in every
// repo of a workspace: the loop itself. Where *intent* lives differs between
// a hub and a spoke, so that paragraph is appended separately — the rendered
// hub text is byte-identical to the single constant this was split out of,
// which is what keeps re-running adopt a no-op on already-wired repos.
const agreementSteps = beginMark + `
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
`

// hubIntent closes the agreement in the repo that carries .truthboard/.
const hubIntent = `
Story *intent* (title, goal, acceptance, epic, priority, scope) is always
editable — ` + "`update_spec`" + ` over MCP, the CLI, or editing
` + "`.truthboard/specs/*.md`" + ` directly. Commit intent changes like code.
`

// agreement is the hub's full working agreement, unchanged in wording from
// the version that shipped as one constant.
const agreement = agreementSteps + hubIntent

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

// spokeIntent closes the agreement in a spoke: the loop above is identical,
// but the specs are not here. An agent told to edit `.truthboard/specs/*.md`
// in a repo that has no such directory would either create one — making a
// second, competing hub — or conclude the tooling is broken.
const spokeIntent = `
### This repo is a workspace spoke

Intent lives in the hub (` + "`%s`" + `), not here — every spec, epic and sprint of
the workspace, which gathers proof from %s. The MCP server
registered in this repo already points there, so ` + "`get_brief`" + `, ` + "`next_spec`" + `
and ` + "`update_spec`" + ` work from this directory unchanged, and there is no
` + "`.truthboard/`" + ` here to look for.

What belongs in this repo is the work itself: a branch carrying the spec id
and the ` + "`Spec: tb-1234`" + ` trailer on every commit. Ids are global across the
workspace, so a brief handed out anywhere is claimed by that trailer here —
and a story whose acceptance spans repos is split (or declared with
` + "`repos:`" + `) in the hub, one provable landing per spec.
`

// agreementText renders the working agreement, appending workspace
// guidance when a manifest declares spokes. upsertBlock replaces the whole
// marker block, so adding a workspace later just means re-running adopt.
func agreementText(ws *workspace.Workspace) string {
	if ws == nil || len(ws.Repos) == 0 {
		return agreement + endMark + "\n"
	}
	return agreement + fmt.Sprintf(workspaceGuidance, repoNames(ws)) + endMark + "\n"
}

// spokeAgreementText renders the agreement for a spoke, naming the hub by
// the same relative path its MCP registration uses.
func spokeAgreementText(ws *workspace.Workspace, hubArg string) string {
	return agreementSteps + fmt.Sprintf(spokeIntent, hubArg, repoNames(ws)) + endMark + "\n"
}

// repoNames renders the declared spokes as an inline code list.
func repoNames(ws *workspace.Workspace) string {
	names := make([]string, len(ws.Repos))
	for i, r := range ws.Repos {
		names[i] = "`" + r.Name + "`"
	}
	return strings.Join(names, ", ")
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
    .truthboard/*|.mcp.json|.vscode/mcp.json|AGENTS.md|CLAUDE.md) tb_state=governed ;;
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
` + hookEndMark + "\n"

const hookScript = "#!/bin/sh\n" + hookNudge + "exit 0\n"

// legacyNudges are the nudge texts shipped before hookEndMark existed, newest
// first. Without a closing marker an installed nudge cannot be bounded, so an
// upgrade has to match one of these exactly — proof that truthboard wrote it.
// A nudge matching none of them is left alone: silently rewriting a hook we
// cannot prove we authored risks destroying someone else's script.
var legacyNudges = []string{
	// v0.8.2 (tb-3d43): taught the nudge to stay quiet on intent-only commits.
	hookMark + ` — warns when a commit has no Spec trailer; NEVER blocks.
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
`,
	// tb-22b8, the original: warned on every trailerless commit.
	hookMark + ` — warns when a commit has no Spec trailer; NEVER blocks.
tb_msg=$(cat "$1")
case "$tb_msg" in
  Merge*|Revert*|fixup!*|squash!*) : ;;
  *)
    if ! printf '%s\n' "$tb_msg" | grep -qE '^Spec: tb-'; then
      echo "truthboard: no 'Spec: <id>' trailer — this commit will show up as shadow work (truthboard audit)" >&2
    fi ;;
esac
`,
}

// Agents performs the wiring and returns a human-readable action log.
func Agents(repo string, hooks bool) ([]string, error) {
	// A malformed manifest must not block adoption; the audit already
	// reports it loudly, so the agreement simply omits the guidance.
	ws, _ := workspace.Load(repo)
	return wire(repo, hooks, "", agreementText(ws), ws)
}

// wire writes the agent-facing files into one repository. hubArg is the
// repository the MCP server should serve, relative to this one — empty when
// this repo carries .truthboard/ itself. agreement is the block to install,
// already rendered, because a hub and a spoke say different things about
// where intent lives.
func wire(repo string, hooks bool, hubArg, agreement string, ws *workspace.Workspace) ([]string, error) {
	var log []string
	step := func(format string, a ...any) { log = append(log, fmt.Sprintf(format, a...)) }

	// Only a hub gets a specs directory. Creating one in a spoke would make
	// it look like a second hub — two competing sources of intent, which is
	// the one thing the hub-and-spokes model exists to prevent.
	mcpArgs := []string{"mcp"}
	if hubArg == "" {
		if err := os.MkdirAll(filepath.Join(repo, ".truthboard", "specs"), 0o755); err != nil {
			return nil, err
		}
	} else {
		mcpArgs = append(mcpArgs, hubArg)
	}

	// Two files, one server: `.mcp.json` is what Claude Code, Cursor and
	// friends read; `.vscode/mcp.json` is what VS Code — and so GitHub
	// Copilot — reads. A hub's entry carries no repo argument (the file is
	// committed and shared, and the workspace layout's `mcp ./hub` is a
	// documented manual edit); a spoke's must, since serving the spoke
	// itself would serve a repository with no specs in it.
	changed, err := registerMCP(filepath.Join(repo, ".mcp.json"), "mcpServers",
		map[string]any{"command": "truthboard", "args": mcpArgs})
	if err != nil {
		return nil, err
	}
	if changed {
		step(".mcp.json: registered the truthboard MCP server")
	} else {
		step(".mcp.json: truthboard already registered")
	}

	vscodeChanged, err := registerMCP(filepath.Join(repo, ".vscode", "mcp.json"), vscodeMCPKey,
		map[string]any{"type": "stdio", "command": "truthboard", "args": mcpArgs})
	if err != nil {
		return nil, err
	}
	if vscodeChanged {
		step(".vscode/mcp.json: registered the truthboard MCP server (VS Code / GitHub Copilot)")
	} else {
		step(".vscode/mcp.json: truthboard already registered")
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

	agentsPath := filepath.Join(repo, "AGENTS.md")
	if changed, err = upsertBlock(agentsPath, agreement, true); err != nil {
		return nil, err
	}
	step("AGENTS.md: working agreement %s", writtenWord(changed))
	switch {
	case hubArg != "":
		step("AGENTS.md: names the hub (%s) as where intent lives", hubArg)
	case ws != nil && len(ws.Repos) > 0:
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

// vscodeMCPKey is VS Code's top-level key for MCP servers — GitHub Copilot
// reads `.vscode/mcp.json`, and that schema spells the collection `servers`
// (not `mcpServers`) and wants the transport named explicitly. Same binary,
// same subcommand, different spelling: registering both files is what makes
// "point your tool at the server" true for a Copilot shop too.
const vscodeMCPKey = "servers"

// registerMCP adds the truthboard server to an MCP config file under the
// given top-level key, preserving any other servers and unknown keys. entry
// is the server object to write, so each client gets the shape it expects.
func registerMCP(path, key string, entry map[string]any) (changed bool, err error) {
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("%s exists but is not valid JSON: %w", path, err)
		}
	}
	servers, _ := doc[key].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if _, ok := servers["truthboard"]; ok {
		return false, nil
	}
	servers["truthboard"] = entry
	doc[key] = servers
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return false, err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return false, err
		}
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
	content := string(raw)
	if begin := strings.Index(content, hookMark); begin >= 0 {
		return upgradeNudge(path, content, begin)
	}
	// Existing hook belongs to someone else: insert the exit-code-neutral
	// nudge right after its shebang so the hook's own logic still decides
	// the outcome — we only ever print.
	if nl := strings.Index(content, "\n"); strings.HasPrefix(content, "#!") && nl > 0 {
		content = content[:nl+1] + "\n" + hookNudge + "\n" + content[nl+1:]
	} else {
		content = hookNudge + "\n" + content
	}
	return "inserted into your existing commit-msg hook (warn-only)", os.WriteFile(path, []byte(content), 0o755)
}

// upgradeNudge replaces an already-installed nudge with the current one,
// touching nothing else in the hook. A nudge bounded by hookEndMark is swapped
// directly; an older unbounded one has to match legacyNudges exactly before it
// can be replaced. Anything else is reported and left exactly as it is —
// truthboard does not rewrite a block it cannot prove it wrote.
func upgradeNudge(path, content string, begin int) (string, error) {
	if end := strings.Index(content[begin:], hookEndMark); end >= 0 {
		stop := begin + end + len(hookEndMark)
		return swapNudge(path, content, begin, stop)
	}
	for _, old := range legacyNudges {
		if i := strings.Index(content, old); i >= 0 {
			return swapNudge(path, content, i, i+len(old))
		}
	}
	return "left alone — the nudge here is not one truthboard wrote, so it is outdated" +
		" but safe: to refresh it, delete the block by hand (or the whole hook if it is" +
		" only truthboard's) and re-run", nil
}

// swapNudge writes the current nudge over content[begin:stop], preserving the
// hook's own lines on either side and reporting an identical nudge as a no-op
// so a re-run never rewrites the file.
func swapNudge(path, content string, begin, stop int) (string, error) {
	nudge := strings.TrimSuffix(hookNudge, "\n")
	if content[begin:stop] == nudge {
		return "already up to date", nil
	}
	updated := content[:begin] + nudge + content[stop:]
	return "upgraded to the current nudge", os.WriteFile(path, []byte(updated), 0o755)
}
