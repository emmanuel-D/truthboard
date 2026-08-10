package adopt

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoWithIgnore returns a fresh repository whose .gitignore carries rules.
func repoWithIgnore(t *testing.T, rules ...string) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if len(rules) > 0 {
		body := strings.Join(rules, "\n") + "\n"
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func joined(lines []string) string { return strings.Join(lines, "\n") }

func TestAdoptionWarnsWhereItClaimsTheWrite(t *testing.T) {
	repo := repoWithIgnore(t, ".vscode/*")
	log, err := Agents(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	// The warning has to sit against the line claiming the write, not in a
	// trailing summary a reader has already scrolled past.
	var claim, warn = -1, -1
	for i, line := range log {
		if strings.HasPrefix(line, ".vscode/mcp.json: registered") {
			claim = i
		}
		if strings.Contains(line, "⚠ .vscode/mcp.json is written") {
			warn = i
		}
	}
	if claim < 0 || warn < 0 {
		t.Fatalf("want both the write and its warning, got:\n%s", joined(log))
	}
	if warn != claim+1 {
		t.Errorf("warning at %d, write claimed at %d — must be adjacent:\n%s", warn, claim, joined(log))
	}
}

func TestWarningNamesTheRuleThatDoesIt(t *testing.T) {
	repo := repoWithIgnore(t, "# a comment", "build/", ".vscode/*")
	warn := joined(ignoreWarning(repo, ".vscode/mcp.json"))
	for _, want := range []string{".gitignore:3", ".vscode/*"} {
		if !strings.Contains(warn, want) {
			t.Errorf("warning must name the rule and its line, missing %q:\n%s", want, warn)
		}
	}
}

// The suggested exception has to work for the pattern's shape. Git never
// descends into an excluded directory, so a lone negation under `.vscode/`
// is a line that looks like a fix and does nothing.
func TestSuggestedExceptionMatchesThePatternShape(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule string
		want []string
	}{
		{"directory excluded", ".vscode/", []string{"!.vscode/", ".vscode/*", "!.vscode/mcp.json"}},
		{"contents excluded", ".vscode/*", []string{"!.vscode/mcp.json"}},
		{"bare name excludes the directory too", ".vscode", []string{"!.vscode/", ".vscode/*", "!.vscode/mcp.json"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := repoWithIgnore(t, tc.rule)
			// The file is on disk before anything asks about it, as it is in
			// every real call: git reads the filesystem to tell a directory
			// from a file, and the answer for `.vscode/` depends on it.
			if err := os.MkdirAll(filepath.Join(repo, ".vscode"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, ".vscode", "mcp.json"), []byte("{}\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			got := exception(repo, ".vscode/mcp.json")
			if joined(got) != joined(tc.want) {
				t.Errorf("exception for %q = %v, want %v", tc.rule, got, tc.want)
			}
			assertReincludes(t, repo, ".vscode/mcp.json", got)
		})
	}
}

func TestTopLevelFileNeedsOnlyItsOwnNegation(t *testing.T) {
	repo := repoWithIgnore(t, ".mcp.json")
	got := exception(repo, ".mcp.json")
	if joined(got) != "!.mcp.json" {
		t.Errorf("exception = %v, want [!.mcp.json]", got)
	}
	assertReincludes(t, repo, ".mcp.json", got)
}

// assertReincludes proves the suggestion against git itself: appending it to
// the ignore file must actually stop git ignoring the path. A fix that
// silently does nothing would be worse than no suggestion at all.
func assertReincludes(t *testing.T, repo, rel string, lines []string) {
	t.Helper()
	path := filepath.Join(repo, ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, []byte(strings.Join(lines, "\n")+"\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ignored := ignoredBy(repo, rel); ignored {
		t.Errorf("%s is still ignored after applying the suggested fix %v", rel, lines)
	}
}

func TestATrackedFileIsNeverWarnedAbout(t *testing.T) {
	repo := repoWithIgnore(t, ".mcp.json")
	if err := os.WriteFile(filepath.Join(repo, ".mcp.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// -f because the rule excludes it: this is exactly the repo that
	// committed the file first and added the rule later.
	if out, err := exec.Command("git", "-C", repo, "add", "-f", ".mcp.json").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if warn := ignoreWarning(repo, ".mcp.json"); warn != nil {
		t.Errorf("ignore rules do not apply to tracked files, got:\n%s", joined(warn))
	}
}

func TestAdoptionIsSilentAndSucceedsWithoutRules(t *testing.T) {
	repo := repoWithIgnore(t)
	log, err := Agents(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(joined(log), "⚠") && strings.Contains(joined(log), "excludes it") {
		t.Errorf("a repo with no rules on these paths must see no change:\n%s", joined(log))
	}
}

// Warn-only, same doctrine as the spawn and not-a-git-repo warnings: every
// file still lands and the caller still succeeds.
func TestWarningNeverFailsAdoption(t *testing.T) {
	repo := repoWithIgnore(t, ".vscode/", "AGENTS.md")
	if _, err := Agents(repo, false); err != nil {
		t.Fatalf("adoption must not fail over an ignored write: %v", err)
	}
	for _, f := range []string{".mcp.json", ".vscode/mcp.json", "AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(f))); err != nil {
			t.Errorf("%s must still be written: %v", f, err)
		}
	}
}

func TestADirectoryThatIsNotARepositoryIsNotProbed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".mcp.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if warn := ignoreWarning(dir, ".mcp.json"); warn != nil {
		t.Errorf("no repository, no ignore rules to report, got:\n%s", joined(warn))
	}
}

func TestDriftReportsWiringTheSpokeWillThrowAway(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Agents(hub, false); err != nil {
		t.Fatal(err)
	}
	// api excludes .vscode/* in its own repo — the hub's rules say nothing
	// about a spoke's.
	api := filepath.Join(hub, "..", "api")
	if err := os.WriteFile(filepath.Join(api, ".gitignore"), []byte(".vscode/*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Spokes(hub, false); err != nil {
		t.Fatal(err)
	}

	gaps := Gaps(hub)
	var found string
	for _, g := range gaps {
		if strings.Contains(g, ".vscode/mcp.json is wired here but excluded") {
			found = g
		}
	}
	if found == "" {
		t.Fatalf("want the ignored wiring reported as drift, got %v", gaps)
	}
	for _, want := range []string{"api", "../api", ".gitignore:1", "fresh clone"} {
		if !strings.Contains(found, want) {
			t.Errorf("finding missing %q: %s", want, found)
		}
	}
	// Re-running adoption cannot fix a file the repo already has and still
	// throws away, so this finding must not name that as the remedy.
	if strings.Contains(found, "re-run `truthboard init --workspace`") {
		t.Errorf("ignored wiring must not point at the re-run, which cannot fix it: %s", found)
	}
	if strings.Contains(found, "no MCP registration") {
		t.Errorf("the file is present, not missing: %s", found)
	}
}

func TestAWiredSpokeWithNoRulesStaysClean(t *testing.T) {
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	if _, err := Agents(hub, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Spokes(hub, true); err != nil {
		t.Fatal(err)
	}
	if gaps := Gaps(hub); len(gaps) != 0 {
		t.Errorf("no ignore rules, no findings, got %v", gaps)
	}
}
