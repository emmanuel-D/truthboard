package adopt

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func TestAgentsWiresFreshRepo(t *testing.T) {
	repo := gitRepo(t)
	log, err := Agents(repo, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	for _, want := range []string{"registered the truthboard MCP server", "working agreement written", "agreement import written", "installed"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log missing %q:\n%s", want, joined)
		}
	}

	var mcp struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &mcp); err != nil {
		t.Fatal(err)
	}
	if s, ok := mcp.Servers["truthboard"]; !ok || s.Command != "truthboard" || len(s.Args) != 1 || s.Args[0] != "mcp" {
		t.Errorf(".mcp.json = %+v, want truthboard mcp server", mcp.Servers)
	}

	agents, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Spec: tb-1234", "get_brief", "never typed"} {
		if !strings.Contains(string(agents), want) {
			t.Errorf("AGENTS.md missing %q", want)
		}
	}

	// Claude Code loads CLAUDE.md but not AGENTS.md, so adoption must
	// create it and import the agreement.
	claude, err := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should be created on a fresh repo: %v", err)
	}
	if !strings.Contains(string(claude), "@AGENTS.md") {
		t.Errorf("CLAUDE.md missing the @AGENTS.md import:\n%s", claude)
	}

	hook := filepath.Join(repo, ".git", "hooks", "commit-msg")
	info, err := os.Stat(hook)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Error("commit-msg hook is not executable")
	}
}

func TestAgentsUpgradesStalePointerBlock(t *testing.T) {
	repo := gitRepo(t)
	stale := "# My project\n\n" + beginMark + "\n## Task tracking\n\nOld pointer without the import.\n" + endMark + "\n"
	os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(stale), 0o644)

	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	claude, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if !strings.Contains(string(claude), "# My project") {
		t.Errorf("CLAUDE.md lost the owner's content:\n%s", claude)
	}
	if !strings.Contains(string(claude), "@AGENTS.md") || strings.Contains(string(claude), "Old pointer") {
		t.Errorf("stale block should be replaced with the import:\n%s", claude)
	}
	if n := strings.Count(string(claude), beginMark); n != 1 {
		t.Errorf("CLAUDE.md has %d truthboard blocks, want exactly 1", n)
	}
}

func TestAgentsIsIdempotent(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md"))

	log, err := Agents(repo, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	// The hook reports a no-op the same way the other idempotent writers do.
	for _, want := range []string{"already registered", "already up to date"} {
		if !strings.Contains(joined, want) {
			t.Errorf("second run should be a no-op, log:\n%s", joined)
		}
	}
	second, _ := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if string(first) != string(second) {
		t.Error("AGENTS.md changed on second run")
	}
	if n := strings.Count(string(second), beginMark); n != 1 {
		t.Errorf("AGENTS.md has %d truthboard blocks, want exactly 1", n)
	}
}

func TestAgentsPreservesOthersFiles(t *testing.T) {
	repo := gitRepo(t)
	// Pre-existing .mcp.json with another server, CLAUDE.md with content,
	// and a commit-msg hook that blocks — all must survive.
	os.WriteFile(filepath.Join(repo, ".mcp.json"),
		[]byte(`{"mcpServers":{"other":{"command":"other-tool"}}}`), 0o644)
	os.WriteFile(filepath.Join(repo, "CLAUDE.md"),
		[]byte("# My project\n\nBuild with make.\n"), 0o644)
	os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"),
		[]byte("#!/bin/sh\ngrep -q JIRA \"$1\" || exit 1\n"), 0o755)

	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if !strings.Contains(string(raw), "other-tool") || !strings.Contains(string(raw), "truthboard") {
		t.Errorf(".mcp.json lost a server:\n%s", raw)
	}
	claude, _ := os.ReadFile(filepath.Join(repo, "CLAUDE.md"))
	if !strings.Contains(string(claude), "Build with make.") || !strings.Contains(string(claude), "Truthboard") {
		t.Errorf("CLAUDE.md content wrong:\n%s", claude)
	}
	hook, _ := os.ReadFile(filepath.Join(repo, ".git", "hooks", "commit-msg"))
	if !strings.Contains(string(hook), "JIRA") || !strings.Contains(string(hook), hookMark) {
		t.Errorf("existing hook logic lost:\n%s", hook)
	}
	// The nudge must sit before the blocking logic and never add an exit.
	if strings.Index(string(hook), hookMark) > strings.Index(string(hook), "JIRA") {
		t.Error("nudge should be inserted before the existing hook logic")
	}
	if strings.Contains(strings.TrimPrefix(string(hook), hookScript), "exit 0") &&
		strings.Count(string(hook), "exit 0") > strings.Count("#!/bin/sh\ngrep -q JIRA \"$1\" || exit 1\n", "exit 0") {
		t.Error("insertion must not add exit statements to someone else's hook")
	}
}

// vscodeMCP reads the VS Code MCP config a Copilot user's editor picks up.
// Its schema differs from .mcp.json — `servers`, with an explicit transport —
// so it gets its own decoder rather than sharing the Claude-shaped one.
func vscodeMCP(t *testing.T, repo string) map[string]struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
} {
	t.Helper()
	var doc struct {
		Servers map[string]struct {
			Type    string   `json:"type"`
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"servers"`
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".vscode", "mcp.json"))
	if err != nil {
		t.Fatalf(".vscode/mcp.json not written: %v", err)
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf(".vscode/mcp.json is not valid JSON: %v\n%s", err, raw)
	}
	return doc.Servers
}

// GitHub Copilot reads .vscode/mcp.json, so adoption must write it — under
// VS Code's `servers` key, not the `mcpServers` one .mcp.json uses.
func TestAgentsWiresVSCodeMCPForCopilot(t *testing.T) {
	repo := gitRepo(t)
	log, err := Agents(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := vscodeMCP(t, repo)["truthboard"]
	if !ok {
		t.Fatal(".vscode/mcp.json has no truthboard server")
	}
	if s.Type != "stdio" || s.Command != "truthboard" || len(s.Args) != 1 || s.Args[0] != "mcp" {
		t.Errorf("server = %+v, want stdio truthboard mcp", s)
	}
	// The file is committed and shared, so it must not pin this machine's
	// paths — the workspace `mcp ./hub` argument stays a manual edit.
	raw, _ := os.ReadFile(filepath.Join(repo, ".vscode", "mcp.json"))
	if strings.Contains(string(raw), repo) {
		t.Errorf(".vscode/mcp.json leaked an absolute path:\n%s", raw)
	}
	if joined := strings.Join(log, "\n"); !strings.Contains(joined, "Copilot") {
		t.Errorf("log should name the tool this wires, got:\n%s", joined)
	}
}

func TestAgentsPreservesOtherVSCodeServers(t *testing.T) {
	repo := gitRepo(t)
	os.MkdirAll(filepath.Join(repo, ".vscode"), 0o755)
	os.WriteFile(filepath.Join(repo, ".vscode", "mcp.json"),
		[]byte(`{"inputs":[{"id":"tok"}],"servers":{"other":{"command":"other-tool"}}}`), 0o644)

	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	servers := vscodeMCP(t, repo)
	if _, ok := servers["other"]; !ok {
		t.Error(".vscode/mcp.json lost the adopter's own server")
	}
	if _, ok := servers["truthboard"]; !ok {
		t.Error("truthboard was not added")
	}
	// Unknown top-level keys (VS Code's `inputs`) survive the rewrite.
	raw, _ := os.ReadFile(filepath.Join(repo, ".vscode", "mcp.json"))
	if !strings.Contains(string(raw), "inputs") {
		t.Errorf(".vscode/mcp.json lost an unknown top-level key:\n%s", raw)
	}

	log, err := Agents(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(log, "\n"), ".vscode/mcp.json: truthboard already registered") {
		t.Errorf("second run should be a no-op:\n%s", strings.Join(log, "\n"))
	}
}

// A hand-edited config that no longer parses must stop adoption by name,
// never be silently replaced — the same contract .mcp.json already has.
func TestAgentsFailsLoudlyOnMalformedVSCodeMCP(t *testing.T) {
	repo := gitRepo(t)
	os.MkdirAll(filepath.Join(repo, ".vscode"), 0o755)
	broken := []byte("{ not json")
	os.WriteFile(filepath.Join(repo, ".vscode", "mcp.json"), broken, 0o644)

	_, err := Agents(repo, false)
	if err == nil {
		t.Fatal("Agents() = nil error, want a failure naming the file")
	}
	if !strings.Contains(err.Error(), "mcp.json") {
		t.Errorf("error must name the file, got: %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(repo, ".vscode", "mcp.json")); string(raw) != string(broken) {
		t.Errorf("malformed file was overwritten:\n%s", raw)
	}
}

func TestSpawnWarningWhenBinaryOnlyInShellProfile(t *testing.T) {
	sysDir := t.TempDir() // empty: plays the role of /usr/local/bin
	goBin := filepath.Join(t.TempDir(), "go", "bin")
	os.MkdirAll(goBin, 0o755)
	exe := filepath.Join(goBin, "truthboard")
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)

	lines := spawnWarning(exe, []string{sysDir})
	if len(lines) == 0 {
		t.Fatal("expected a warning when no system dir has truthboard")
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{exe, "ln -s", sysDir, "silently"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning missing %q:\n%s", want, joined)
		}
	}
}

func TestSpawnWarningQuietWhenSystemDirHasBinary(t *testing.T) {
	sysDir := t.TempDir()
	os.WriteFile(filepath.Join(sysDir, "truthboard"), []byte("#!/bin/sh\n"), 0o755)

	if lines := spawnWarning("/home/u/go/bin/truthboard", []string{sysDir}); lines != nil {
		t.Errorf("expected no warning, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSpawnWarningIgnoresNonExecutable(t *testing.T) {
	sysDir := t.TempDir()
	os.WriteFile(filepath.Join(sysDir, "truthboard"), []byte("not a binary"), 0o644)

	if lines := spawnWarning("/home/u/go/bin/truthboard", []string{sysDir}); len(lines) == 0 {
		t.Error("a non-executable file must not count as resolvable")
	}
}

func TestHookWarnsButNeverBlocks(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(repo, ".git", "hooks", "commit-msg")

	run := func(msg string) (string, error) {
		msgFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")
		os.WriteFile(msgFile, []byte(msg), 0o644)
		out, err := exec.Command("/bin/sh", hook, msgFile).CombinedOutput()
		return string(out), err
	}

	out, err := run("feat: no trailer here")
	if err != nil {
		t.Errorf("hook must never block, got error %v", err)
	}
	if !strings.Contains(out, "shadow work") {
		t.Errorf("expected a warning for missing trailer, got %q", out)
	}

	for _, msg := range []string{"feat: linked\n\nSpec: tb-1234", "Merge branch 'x'", "Revert \"feat: y\""} {
		out, err := run(msg)
		if err != nil || out != "" {
			t.Errorf("message %q should pass silently, got out=%q err=%v", msg, out, err)
		}
	}
}

// TestAgreementCarriesWorkspaceGuidance: a hub with a workspace manifest
// gets decomposition guidance in its working agreement; a plain repo never
// sees it.
func TestAgreementCarriesWorkspaceGuidance(t *testing.T) {
	repo := t.TempDir()
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "Multi-repo workspace") {
		t.Fatal("plain repo must not get workspace guidance")
	}

	if err := os.MkdirAll(filepath.Join(repo, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "repos:\n  api:\n    remote: git@example.com:acme/api.git\n  web:\n    remote: git@example.com:acme/web.git\n"
	if err := os.WriteFile(filepath.Join(repo, ".truthboard", "workspace.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	log, err := Agents(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"Multi-repo workspace", "`api`, `web`", "create_spec", "needs:", "repos: [api, web]", "orphan"} {
		if !strings.Contains(got, want) {
			t.Errorf("agreement missing %q", want)
		}
	}
	if strings.Count(got, "## Truthboard working agreement") != 1 {
		t.Fatal("re-running adopt must replace the block, not duplicate it")
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "decomposition guidance (2 workspace repos)") {
		t.Errorf("adopt log should mention the guidance, got:\n%s", joined)
	}
}

func TestRepoWarningFiresOutsideAGitRepo(t *testing.T) {
	warn := RepoWarning(t.TempDir())
	if warn == nil {
		t.Fatal("RepoWarning() = nil for a plain directory, want the git-init guidance")
	}
	joined := strings.Join(warn, "\n")
	for _, want := range []string{"not a git repository", "git init"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning missing %q:\n%s", want, joined)
		}
	}
}

func TestRepoWarningQuietInsideAGitRepo(t *testing.T) {
	if warn := RepoWarning(gitRepo(t)); warn != nil {
		t.Errorf("RepoWarning() = %v for a git repo, want nil", warn)
	}
}

// A hub scaffolded before git init must still get every file: the warning
// reports the gap, it never aborts the wiring.
func TestAgentsWiresNonRepoAnyway(t *testing.T) {
	dir := t.TempDir()
	if _, err := Agents(dir, false); err != nil {
		t.Fatalf("Agents() in a non-repo: %v", err)
	}
	for _, f := range []string{".mcp.json", filepath.Join(".vscode", "mcp.json"), "AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}
}

// hookPath is where the nudge lives in a fixture repo.
func hookPath(repo string) string {
	return filepath.Join(repo, ".git", "hooks", "commit-msg")
}

// An adopter carrying any nudge truthboard has ever shipped must end up on
// the current one — that is the whole point of tb-d146.
func TestInstallHookUpgradesEveryLegacyNudge(t *testing.T) {
	for i, old := range legacyNudges {
		repo := gitRepo(t)
		if err := os.WriteFile(hookPath(repo), []byte("#!/bin/sh\n"+old+"exit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		msg, err := installHook(repo)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(msg, "upgraded") {
			t.Errorf("legacyNudges[%d]: msg = %q, want an upgrade", i, msg)
		}
		got, _ := os.ReadFile(hookPath(repo))
		if !strings.Contains(string(got), hookEndMark) {
			t.Errorf("legacyNudges[%d]: upgraded hook has no end marker:\n%s", i, got)
		}
		// The upgrade replaces the block rather than appending a second one.
		// Counting markers is the honest check: the newest legacy nudge is a
		// prefix of the current one, so "old text is gone" cannot be asserted.
		if n := strings.Count(string(got), hookEndMark); n != 1 {
			t.Errorf("legacyNudges[%d]: %d end markers, want exactly 1:\n%s", i, n, got)
		}
		if n := strings.Count(string(got), hookMark); n != 2 {
			t.Errorf("legacyNudges[%d]: %d markers, want the begin/end pair only:\n%s", i, n, got)
		}
		if !strings.Contains(string(got), "exit 0") {
			t.Errorf("legacyNudges[%d]: upgrade dropped the rest of the hook:\n%s", i, got)
		}
	}
}

// Every existing adopter carries a bounded nudge whose governed fileset
// predates .vscode/mcp.json. Re-running init must swap it for the current
// one via the end marker — no legacyNudges entry needed, because the block
// is bounded — and their Copilot config commits must go quiet as a result.
func TestInstallHookUpgradesBoundedStaleNudge(t *testing.T) {
	repo := gitRepo(t)
	stale := strings.Replace(hookNudge, "|.vscode/mcp.json", "", 1)
	if stale == hookNudge {
		t.Fatal("fixture is not stale — the governed fileset no longer names .vscode/mcp.json")
	}
	if err := os.WriteFile(hookPath(repo), []byte("#!/bin/sh\n"+stale+"exit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := installHook(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "upgraded") {
		t.Errorf("msg = %q, want an upgrade", msg)
	}
	got, _ := os.ReadFile(hookPath(repo))
	if !strings.Contains(string(got), ".vscode/mcp.json") {
		t.Errorf("upgraded hook still has the stale fileset:\n%s", got)
	}
	if n := strings.Count(string(got), hookEndMark); n != 1 {
		t.Errorf("%d end markers, want exactly 1:\n%s", n, got)
	}
}

// The nudge may sit inside a hook someone else owns. Their lines are not ours
// to touch — destroying them would be far worse than a stale nudge.
func TestInstallHookUpgradePreservesForeignLines(t *testing.T) {
	repo := gitRepo(t)
	before := "#!/bin/sh\necho \"my own check\"\n\n"
	after := "\nexec /usr/local/bin/lint-commit \"$1\"\n"
	if err := os.WriteFile(hookPath(repo), []byte(before+legacyNudges[len(legacyNudges)-1]+after), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := installHook(repo); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(hookPath(repo))
	if !strings.HasPrefix(string(got), before) {
		t.Errorf("lines before the nudge were altered:\n%s", got)
	}
	if !strings.HasSuffix(string(got), after) {
		t.Errorf("lines after the nudge were altered:\n%s", got)
	}
}

// A block truthboard cannot prove it wrote is reported, never rewritten.
func TestInstallHookLeavesUnrecognisedNudgeAlone(t *testing.T) {
	repo := gitRepo(t)
	foreign := "#!/bin/sh\n" + hookMark + " — hand-edited by the team\nexit 0\n"
	if err := os.WriteFile(hookPath(repo), []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	msg, err := installHook(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "left alone") {
		t.Errorf("msg = %q, want it reported as left alone", msg)
	}
	got, _ := os.ReadFile(hookPath(repo))
	if string(got) != foreign {
		t.Errorf("an unrecognised nudge was rewritten:\ngot:\n%s\nwant:\n%s", got, foreign)
	}
}

// A current nudge must not be rewritten — a no-op leaves the file untouched.
func TestInstallHookCurrentNudgeIsANoOp(t *testing.T) {
	repo := gitRepo(t)
	if _, err := installHook(repo); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(hookPath(repo))
	msg, err := installHook(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "already up to date") {
		t.Errorf("msg = %q, want a no-op", msg)
	}
	second, _ := os.ReadFile(hookPath(repo))
	if string(first) != string(second) {
		t.Error("a current hook was rewritten on re-run")
	}
	if n := strings.Count(string(second), hookMark); n != 2 {
		t.Errorf("hook has %d markers, want exactly the begin/end pair", n)
	}
}

// TestAgreementAsksForTheAcceptanceTick guards the step whose absence let
// stories land with an untouched checklist: the agreement every adopted
// repo carries must name the verb, and re-running adopt must not stutter.
func TestAgreementAsksForTheAcceptanceTick(t *testing.T) {
	repo := t.TempDir()
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"check_acceptance", "truthboard check tb-1234", "Never tick what you did not\n   verify"} {
		if !strings.Contains(got, want) {
			t.Errorf("agreement missing %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "check_acceptance") != 1 {
		t.Error("re-running adopt duplicated the tick step instead of replacing the block")
	}
}
