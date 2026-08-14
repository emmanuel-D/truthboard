package adopt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testJetbrains is the config location under a temp dir, so no test in this
// file ever reads or writes the real $HOME.
func testJetbrains(t *testing.T) jetbrains {
	t.Helper()
	cfg, ok := jetbrainsConfigFor("darwin", t.TempDir())
	if !ok {
		t.Fatal("a Unix home must resolve a config location")
	}
	return cfg
}

// tempHome points os.UserHomeDir — and so the adoption path itself — at an
// empty directory, so a test of the whole command still never reads the
// developer's own config, and never depends on whether they use IntelliJ.
func tempHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// writeConfig puts body at the config location, creating its parents.
func writeConfig(t *testing.T, j jetbrains, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(j.config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.config, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkIdea creates a .idea directory at rel inside dir.
func mkIdea(t *testing.T, dir, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, rel, ".idea"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestJetbrainsWarningNamesTheFileAndTheSnippet(t *testing.T) {
	repo := t.TempDir()
	mkIdea(t, repo, ".")
	warn := joined(testJetbrains(t).warning(repo, "/w/hub"))

	for _, want := range []string{
		"~/.config/github-copilot/intellij/mcp.json",
		`"servers"`,
		`"type": "stdio"`,
		`"command": "truthboard"`,
		`"/w/hub"`,
	} {
		if !strings.Contains(warn, want) {
			t.Errorf("warning must carry %q — it is the whole fix:\n%s", want, warn)
		}
	}
}

// Silence is the default. A warning every adopter sees is one every adopter
// learns to skim past, and then spawnWarning and ignoreWarning go unread too.
func TestJetbrainsSilentWithoutAnIdeaDirectory(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "internal", "adopt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if lines := testJetbrains(t).warning(repo, repo); lines != nil {
		t.Errorf("no .idea/ anywhere, want silence:\n%s", joined(lines))
	}
}

// A module opened as its own project in a monorepo is still a JetBrains user.
func TestJetbrainsFindsIdeaInASubdirectory(t *testing.T) {
	repo := t.TempDir()
	mkIdea(t, repo, filepath.Join("services", "api"))
	if lines := testJetbrains(t).warning(repo, repo); len(lines) == 0 {
		t.Error("a .idea/ below the root is still IntelliJ, want the warning")
	}
}

// The check reads the filesystem, never git: .idea/ is very often gitignored,
// and an ignored .idea/ is still a developer using IntelliJ.
func TestJetbrainsWarnsThroughGitignore(t *testing.T) {
	repo := repoWithIgnore(t, ".idea/")
	mkIdea(t, repo, ".")
	if lines := testJetbrains(t).warning(repo, repo); len(lines) == 0 {
		t.Error("an ignored .idea/ is still IntelliJ, want the warning")
	}
}

func TestJetbrainsQuietWhenAlreadyRegistered(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"vscode schema", `{"servers":{"truthboard":{"type":"stdio","command":"truthboard"}}}`},
		{"hand-written mcpServers", `{"mcpServers":{"truthboard":{"command":"truthboard"}}}`},
		{"alongside another server", `{"servers":{"other":{"command":"x"},"truthboard":{"command":"truthboard"}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			mkIdea(t, repo, ".")
			j := testJetbrains(t)
			writeConfig(t, j, tc.body)
			if lines := j.warning(repo, repo); lines != nil {
				t.Errorf("a wired machine must not be nagged:\n%s", joined(lines))
			}
		})
	}
}

// Absent, unreadable and malformed all mean "not wired": this step can never
// fail a wiring, and never rewrites a file it could not parse.
func TestJetbrainsWarnsOnUnusableConfig(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T, jetbrains)
	}{
		{"absent", func(*testing.T, jetbrains) {}},
		{"malformed JSON", func(t *testing.T, j jetbrains) { writeConfig(t, j, `{"servers": {`) }},
		{"servers is not an object", func(t *testing.T, j jetbrains) { writeConfig(t, j, `{"servers": "nope"}`) }},
		{"registers something else", func(t *testing.T, j jetbrains) {
			writeConfig(t, j, `{"servers":{"other":{"command":"x"}}}`)
		}},
		{"unreadable", func(t *testing.T, j jetbrains) {
			writeConfig(t, j, `{"servers":{"truthboard":{}}}`)
			if err := os.Chmod(j.config, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(j.config, 0o644) })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if os.Geteuid() == 0 && tc.name == "unreadable" {
				t.Skip("root reads anything")
			}
			repo := t.TempDir()
			mkIdea(t, repo, ".")
			j := testJetbrains(t)
			tc.setup(t, j)

			before, err := os.ReadFile(j.config)
			if lines := j.warning(repo, repo); len(lines) == 0 {
				t.Error("an unusable config is not a wired one, want the warning")
			}
			// Never writes: the config is only ever read.
			after, errAfter := os.ReadFile(j.config)
			if (err == nil) != (errAfter == nil) || string(before) != string(after) {
				t.Error("the config must be read, never written")
			}
		})
	}
}

// The warning has to travel with the wiring, so a workspace reports it inside
// the step log of the spoke it is true about — like ignoreWarning, never as a
// trailing summary about "some repo".
func TestAdoptionWarnsAboutJetbrainsWhereItWiresTheRepo(t *testing.T) {
	tempHome(t)
	if _, ok := jetbrainsConfig(); !ok {
		t.Skip("no advisable config location on this platform")
	}
	repo := t.TempDir()
	mkIdea(t, repo, ".")
	log, err := Agents(repo, false)
	if err != nil {
		t.Fatalf("a JetBrains repo must still adopt cleanly: %v", err)
	}

	var claim, warn = -1, -1
	for i, line := range log {
		if strings.HasPrefix(line, ".vscode/mcp.json:") {
			claim = i
		}
		if strings.Contains(line, ".idea/ directory") {
			warn = i
		}
	}
	if claim < 0 || warn < 0 {
		t.Fatalf("want the VS Code write and the JetBrains warning, got:\n%s", joined(log))
	}
	if warn <= claim {
		t.Errorf("warning at %d must follow the MCP registrations at %d:\n%s", warn, claim, joined(log))
	}
}

// A workspace reports the warning next to the spoke that earned it — and only
// that spoke. Naming the hub absolutely is what makes the snippet paste-able
// into a file shared by every project the IDE opens.
func TestSpokeWarningTravelsWithTheSpoke(t *testing.T) {
	tempHome(t)
	if _, ok := jetbrainsConfig(); !ok {
		t.Skip("no advisable config location on this platform")
	}
	hub := hubWithSpokes(t, twoSpokes, "api", "web")
	mkIdea(t, filepath.Join(hub, "..", "api"), ".")

	log, err := Spokes(hub, false)
	if err != nil {
		t.Fatal(err)
	}
	var current string
	warned := map[string]string{}
	for _, line := range log {
		if strings.HasPrefix(line, "spoke ") {
			current = strings.Fields(line)[1]
		}
		if strings.Contains(line, ".idea/ directory") {
			warned[current] = line
		}
		if strings.Contains(line, `"servers"`) {
			warned[current+" snippet"] = line
		}
	}
	if _, ok := warned["api"]; !ok {
		t.Errorf("the spoke carrying .idea/ must be warned in its own step log:\n%s", joined(log))
	}
	if line, ok := warned["web"]; ok {
		t.Errorf("a spoke without .idea/ must stay silent, got %q", line)
	}
	hubAbs, err := filepath.Abs(hub)
	if err != nil {
		t.Fatal(err)
	}
	if snippet := warned["api snippet"]; !strings.Contains(snippet, hubAbs) || strings.Contains(snippet, "../hub") {
		t.Errorf("snippet must name the hub absolutely (%s), got:\n%s", hubAbs, snippet)
	}
}

// Windows keeps that config somewhere other than ~/.config, so the answer
// there is silence — spawnWarning's precedent. Sending a Windows user to
// create a Unix path is worse than saying nothing: they would write a file
// the IDE never reads and believe themselves wired.
func TestJetbrainsSaysNothingItCannotStandBehind(t *testing.T) {
	if _, ok := jetbrainsConfigFor("windows", `C:\Users\dev`); ok {
		t.Error("windows has no advisable location yet, want silence")
	}
	if _, ok := jetbrainsConfigFor("linux", ""); ok {
		t.Error("no home means no path to name, want silence")
	}
	cfg, ok := jetbrainsConfigFor("darwin", "/Users/dev")
	if !ok {
		t.Fatal("a Unix home must resolve a config location")
	}
	if want := "~/.config/github-copilot/intellij/mcp.json"; cfg.display != want {
		t.Errorf("display = %q, want %q — the README names this exact path", cfg.display, want)
	}
	if want := "/Users/dev/.config/github-copilot/intellij/mcp.json"; cfg.config != want {
		t.Errorf("config = %q, want %q", cfg.config, want)
	}
}

// The snippet points at the hub, absolutely: that file is not committed, so
// the relative-path rule that governs .mcp.json does not govern it, and a
// global slot cannot lean on the directory the IDE spawns the server in.
func TestJetbrainsSnippetNamesTheHubAbsolutely(t *testing.T) {
	for _, tc := range []struct {
		name   string
		hubArg string
	}{
		{"hub adopted in place", ""},
		{"spoke pointing up at ./hub", filepath.Join("..", "hub")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := filepath.Join(t.TempDir(), "spoke")
			if err := os.MkdirAll(repo, 0o755); err != nil {
				t.Fatal(err)
			}
			got := hubPath(repo, tc.hubArg)
			if !filepath.IsAbs(got) {
				t.Errorf("hub path %q must be absolute", got)
			}
			if strings.Contains(got, "..") {
				t.Errorf("hub path %q must be resolved, not relative in disguise", got)
			}
		})
	}
}
