package adopt

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ownContent is what an adopter had before truthboard arrived: the file they
// tuned by hand, which uninstall has to give back byte for byte.
const ownContent = "# My project\n\n## Style\n\nUse tabs. Run make test before pushing.\n"

// ownHook is someone else's commit-msg hook — one that decides an outcome,
// which is why the nudge was only ever inserted into it, never over it.
const ownHook = "#!/bin/sh\n# my own hook\ngrep -q \"JIRA-\" \"$1\" || exit 1\nexit 0\n"

func adoptedRepo(t *testing.T) string {
	t.Helper()
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(ownContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "commit-msg"), []byte(ownHook), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	return repo
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestUninstallPlanWritesNothing(t *testing.T) {
	repo := adoptedRepo(t)
	before := map[string]string{}
	for _, f := range []string{"CLAUDE.md", "AGENTS.md", ".mcp.json", ".vscode/mcp.json"} {
		before[f] = read(t, filepath.Join(repo, f))
	}
	hookBefore := read(t, filepath.Join(repo, ".git", "hooks", "commit-msg"))

	log, err := Uninstall(repo, false, false)
	if err != nil {
		t.Fatal(err)
	}
	for f, want := range before {
		if got := read(t, filepath.Join(repo, f)); got != want {
			t.Errorf("%s changed during a plan-only run", f)
		}
	}
	if got := read(t, filepath.Join(repo, ".git", "hooks", "commit-msg")); got != hookBefore {
		t.Error("commit-msg hook changed during a plan-only run")
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "would") {
		t.Errorf("a plan should speak in the conditional:\n%s", joined)
	}
}

func TestUninstallGivesBackYourOwnContent(t *testing.T) {
	repo := adoptedRepo(t)
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo, "CLAUDE.md")); got != ownContent {
		t.Errorf("CLAUDE.md = %q, want the adopter's own content back byte for byte", got)
	}
	// AGENTS.md did not exist before adoption, so nothing of anyone else's is
	// in it — uninstall un-creates what init created.
	if exists(filepath.Join(repo, "AGENTS.md")) {
		t.Error("AGENTS.md survived, but truthboard wrote the whole file")
	}
}

func TestUninstallKeepsSomeoneElsesHook(t *testing.T) {
	repo := adoptedRepo(t)
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(repo, ".git", "hooks", "commit-msg"))
	if strings.Contains(got, hookMark) {
		t.Errorf("nudge survived:\n%s", got)
	}
	for _, want := range []string{"JIRA-", "exit 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("hook lost the adopter's own logic (%q):\n%s", want, got)
		}
	}
	// Byte-identical, not merely equivalent: a hook adopted and abandoned a
	// few times must not accumulate blank lines at the seam.
	if got != ownHook {
		t.Errorf("hook = %q, want the adopter's own hook back exactly", got)
	}
}

func TestUninstallDeletesAHookThatIsOnlyOurs(t *testing.T) {
	repo := gitRepo(t)
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(repo, ".git", "hooks", "commit-msg")) {
		t.Error("hook survived, but truthboard wrote the whole thing")
	}
}

func TestUninstallLeavesANudgeItCannotProveItWrote(t *testing.T) {
	repo := gitRepo(t)
	// A nudge carrying our marker but not our text: a hook someone edited.
	foreign := "#!/bin/sh\n" + hookMark + " — hand-edited by us\necho hi\nexit 0\n"
	path := filepath.Join(repo, ".git", "hooks", "commit-msg")
	if err := os.WriteFile(path, []byte(foreign), 0o755); err != nil {
		t.Fatal(err)
	}
	log, err := Uninstall(repo, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, path); got != foreign {
		t.Errorf("hook was rewritten:\n%s", got)
	}
	if !strings.Contains(strings.Join(log, "\n"), "left alone") {
		t.Errorf("silently skipping is not the same as saying so:\n%s", strings.Join(log, "\n"))
	}
}

func TestUninstallKeepsOtherMCPServers(t *testing.T) {
	repo := gitRepo(t)
	mine := []byte(`{"mcpServers":{"filesystem":{"command":"npx","args":["-y","fs"]}},"other":1}` + "\n")
	if err := os.WriteFile(filepath.Join(repo, ".mcp.json"), mine, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Agents(repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]any `json:"mcpServers"`
		Other   int            `json:"other"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(repo, ".mcp.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if _, gone := doc.Servers["truthboard"]; gone {
		t.Error("truthboard server survived")
	}
	if _, kept := doc.Servers["filesystem"]; !kept {
		t.Error("someone else's MCP server was removed")
	}
	if doc.Other != 1 {
		t.Error("an unknown top-level key was dropped")
	}
	// This one held nothing else, so it goes entirely.
	if exists(filepath.Join(repo, ".vscode", "mcp.json")) {
		t.Error(".vscode/mcp.json survived, but truthboard was the only server in it")
	}
}

func TestUninstallClearsRunStateAndKeepsSpecs(t *testing.T) {
	repo := adoptedRepo(t)
	runDir := filepath.Join(repo, ".git", "truthboard")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "ui.json"), []byte(`{"pid":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	specFile := filepath.Join(repo, ".truthboard", "specs", "tb-0001-x.md")
	if err := os.WriteFile(specFile, []byte("# story\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	log, err := Uninstall(repo, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if exists(runDir) {
		t.Error(".git/truthboard/ survived — it never shows in git status, so nobody would find it")
	}
	if !exists(specFile) {
		t.Error("a story was deleted without --specs")
	}
	if !strings.Contains(strings.Join(log, "\n"), ".truthboard/: kept") {
		t.Error("uninstall should say where the stories still are")
	}
}

func TestUninstallSpecsDeletesThemOnlyWhenAsked(t *testing.T) {
	repo := adoptedRepo(t)
	if _, err := Uninstall(repo, true, true); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(repo, ".truthboard")) {
		t.Error(".truthboard/ survived --specs")
	}
}

func TestUninstallIsQuietOnARepoItNeverTouched(t *testing.T) {
	repo := gitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "CLAUDE.md"), []byte(ownContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(repo, "CLAUDE.md")); got != ownContent {
		t.Errorf("a CLAUDE.md with no truthboard block was rewritten:\n%s", got)
	}
}

func TestUninstallAfterAdoptIsAdoptable(t *testing.T) {
	// The round trip has to be reversible both ways: someone who leaves and
	// comes back should land on the same wiring, not a second copy of it.
	repo := adoptedRepo(t)
	if _, err := Uninstall(repo, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Agents(repo, true); err != nil {
		t.Fatal(err)
	}
	claude := read(t, filepath.Join(repo, "CLAUDE.md"))
	if n := strings.Count(claude, beginMark); n != 1 {
		t.Errorf("CLAUDE.md carries %d truthboard blocks, want 1:\n%s", n, claude)
	}
	if !strings.HasPrefix(claude, ownContent) {
		t.Errorf("the adopter's content did not survive the round trip:\n%s", claude)
	}
}

func TestUninstallRemovesOnlyOurNpmScripts(t *testing.T) {
	repo := npmRepo(t, `{"name":"x","version":"1.0.0","scripts":{"test":"jest"}}`)
	if _, err := NpmScripts(repo); err != nil {
		t.Fatal(err)
	}
	// The adopter has since made board:audit theirs.
	if err := npmPkgSet(repo, "scripts.board:audit", "truthboard audit --status stalled"); err != nil {
		t.Fatal(err)
	}
	log, err := Uninstall(repo, true, false)
	if err != nil {
		t.Fatal(err)
	}
	scripts := readScripts(t, repo)
	if _, gone := scripts["board"]; gone {
		t.Error("board script survived")
	}
	if scripts["board:audit"] != "truthboard audit --status stalled" {
		t.Errorf("board:audit = %q, want the adopter's edited version kept", scripts["board:audit"])
	}
	if scripts["test"] != "jest" {
		t.Error("a script that was never ours was removed")
	}
	if !strings.Contains(strings.Join(log, "\n"), "keeping your edited script") {
		t.Errorf("keeping a script quietly is not keeping it honestly:\n%s", strings.Join(log, "\n"))
	}
}
