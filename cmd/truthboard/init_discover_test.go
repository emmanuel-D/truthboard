package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// workspaceFolder builds the layout adoption is meant to meet: a folder of
// checkouts with no planning repo among them. Returns the folder and the
// path the hub would take inside it.
func workspaceFolder(t *testing.T, names ...string) (root, hub string) {
	t.Helper()
	root = t.TempDir()
	for _, name := range names {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "init", "-b", "main")
		run(t, dir, "config", "user.name", "Test")
		run(t, dir, "config", "user.email", "test@example.com")
		run(t, dir, "remote", "add", "origin", "git@example.com:acme/"+name+".git")
	}
	return root, filepath.Join(root, "hub")
}

// identify gives git an author for the length of the test, wherever it runs.
func identify(t *testing.T) {
	t.Helper()
	for _, v := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(v, "Test")
	}
	for _, v := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(v, "test@example.com")
	}
}

func run(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), repo, err, out)
	}
	return strings.TrimSpace(string(out))
}

// capture runs f with stdout collected.
func capture(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w
	code := f()
	os.Stdout = saved
	w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

func manifest(t *testing.T, hub string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(hub, ".truthboard", "workspace.yml"))
	if err != nil {
		t.Fatalf("no manifest: %v", err)
	}
	return string(raw)
}

// The whole point of the feature: nothing typed, nothing transcribed.
func TestInitWorkspaceDiscoversTheReposBesideIt(t *testing.T) {
	root, hub := workspaceFolder(t, "api", "web")
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	for _, want := range []string{"api", "web", "../api", "../web", "git@example.com:acme/api.git"} {
		if !strings.Contains(out, want) {
			t.Errorf("proposal must show %q:\n%s", want, out)
		}
	}
	m := manifest(t, hub)
	for _, want := range []string{"api:", "web:", "path: ../api", "remote: git@example.com:acme/api.git"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %q:\n%s", want, m)
		}
	}
	// Same wiring as hand-typed pairs, spokes included.
	for _, spoke := range []string{"api", "web"} {
		for _, f := range []string{".mcp.json", "AGENTS.md"} {
			if _, err := os.Stat(filepath.Join(root, spoke, f)); err != nil {
				t.Errorf("%s/%s not wired: %v", spoke, f, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, spoke, ".truthboard")); !os.IsNotExist(err) {
			t.Errorf("%s must not become a second hub", spoke)
		}
	}
}

// A run that cannot ask must not answer for the adopter: declaring a repo
// nobody confirmed is a board gathering proof from something unintended.
func TestNonInteractiveRunDeclaresNothingAndNamesTheFlag(t *testing.T) {
	_, hub := workspaceFolder(t, "api", "web")
	out, code := capture(t, func() int { return runInit([]string{"--workspace", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "--yes") {
		t.Errorf("must name the flag that would have declared them:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(hub, ".truthboard", "workspace.yml")); !os.IsNotExist(err) {
		t.Error("nothing confirmed, nothing declared — a manifest was written anyway")
	}
	// It still showed what it found: that is the teaching half of the prompt.
	if !strings.Contains(out, "api") || !strings.Contains(out, "../web") {
		t.Errorf("the proposal itself must still be printed:\n%s", out)
	}
}

// Explicit pairs are an answer already given.
func TestExplicitPairsSuppressDiscovery(t *testing.T) {
	_, hub := workspaceFolder(t, "api", "web")
	out, code := capture(t, func() int {
		return runInit([]string{"--workspace", "api=git@example.com:acme/api.git", "--path", "api=../api", hub})
	})
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	m := manifest(t, hub)
	if !strings.Contains(m, "api:") {
		t.Errorf("declared pair missing:\n%s", m)
	}
	if strings.Contains(m, "web:") {
		t.Errorf("web was never asked for and must not be declared:\n%s", m)
	}
}

// A re-run offers only what is undeclared, and never rewrites an entry.
func TestRerunOffersOnlyTheUndeclared(t *testing.T) {
	_, hub := workspaceFolder(t, "api", "web")
	if code := runInit([]string{"--workspace", "api=git@example.com:acme/other.git", "--path", "api=../api", hub}); code != 0 {
		t.Fatal("first run failed")
	}
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	m := manifest(t, hub)
	// The hand-declared remote survives: an existing entry is intent, and
	// discovery does not get to correct it.
	if !strings.Contains(m, "acme/other.git") {
		t.Errorf("an existing entry was rewritten:\n%s", m)
	}
	if !strings.Contains(m, "web:") {
		t.Errorf("the undeclared repo should have been offered and taken:\n%s", m)
	}
}

// A hub directory truthboard creates itself has no history to respect.
func TestHubDirectoryTruthboardCreatesIsInitialised(t *testing.T) {
	_, hub := workspaceFolder(t, "api")
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(hub, ".git")); err != nil {
		t.Errorf("the hub it created must be a repository: %v", err)
	}
	if !strings.Contains(out, "git init") {
		t.Errorf("it must say it did so:\n%s", out)
	}
	if strings.Contains(out, "not a git repository yet") {
		t.Errorf("no warning is due for a repo it just created:\n%s", out)
	}
}

// An existing directory is the adopter's, and tb-a4ab's warning stands.
func TestAnExistingNonRepoDirectoryKeepsTheWarning(t *testing.T) {
	_, hub := workspaceFolder(t, "api")
	if err := os.MkdirAll(hub, 0o755); err != nil {
		t.Fatal(err)
	}
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(hub, ".git")); !os.IsNotExist(err) {
		t.Error("truthboard must not git init a directory it did not create")
	}
	if !strings.Contains(out, "not a git repository yet") {
		t.Errorf("the warning must stand:\n%s", out)
	}
}

func TestCommitFlagRecordsTheWiringEverywhereItLanded(t *testing.T) {
	// The hub is created mid-run, so it cannot be configured beforehand and
	// falls back to whatever identity the machine has. A developer laptop has
	// one and a CI runner does not, which is a test that passes locally and
	// fails on push. The environment supplies it instead, for every repo this
	// run commits in.
	identify(t)
	root, hub := workspaceFolder(t, "api", "web")
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", "--commit", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	run(t, hub, "config", "user.name", "Test")
	run(t, hub, "config", "user.email", "test@example.com")
	for _, spoke := range []string{"api", "web"} {
		dir := filepath.Join(root, spoke)
		if n := run(t, dir, "rev-list", "--count", "--all"); n != "1" {
			t.Errorf("%s: %s commits, want the adoption commit", spoke, n)
		}
		if subject := run(t, dir, "log", "-1", "--format=%s"); subject != "Track work with truthboard" {
			t.Errorf("%s: subject = %q", spoke, subject)
		}
	}
}

// A flag taking a separate value must not leave that value in the
// positionals, where it is read as the repo path: `--port 1399` once meant
// "init ./1399", which scaffolded a hub in a directory named after a port.
func TestAFlagValueIsNeverMistakenForTheRepoPath(t *testing.T) {
	root, hub := workspaceFolder(t, "api")
	out, code := capture(t, func() int {
		return runInit([]string{"--workspace", "--yes", "--port", "1399", hub})
	})
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "1399")); !os.IsNotExist(err) {
		t.Error("the port value was taken for a directory to scaffold")
	}
	if _, err := os.Stat(filepath.Join(hub, ".truthboard", "workspace.yml")); err != nil {
		t.Errorf("the hub itself was not scaffolded: %v", err)
	}
}

// One repo that cannot commit must not leave the others uncommitted — and
// the run must still fail, because a commit that was asked for and did not
// happen is not something to discover later.
func TestOneRepoThatCannotCommitDoesNotStopTheRest(t *testing.T) {
	identify(t)
	root, hub := workspaceFolder(t, "api", "web")
	// api refuses every commit made in it.
	hook := filepath.Join(root, "api", ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", "--commit", hub}) })
	if code != 1 {
		t.Errorf("exit = %d, want 1 — a refused commit is not a success\n%s", code, out)
	}
	if n := run(t, filepath.Join(root, "web"), "rev-list", "--count", "--all"); n != "1" {
		t.Errorf("web has %s commits — one repo's failure stopped the others", n)
	}
	if n := run(t, hub, "rev-list", "--count", "--all"); n != "1" {
		t.Errorf("hub has %s commits — one repo's failure stopped the others", n)
	}
}

// Without the flag, the commands are printed and nothing is written.
func TestWithoutTheCommitFlagNothingIsCommitted(t *testing.T) {
	root, hub := workspaceFolder(t, "api")
	out, code := capture(t, func() int { return runInit([]string{"--workspace", "--yes", hub}) })
	if code != 0 {
		t.Fatalf("exit = %d\n%s", code, out)
	}
	if !strings.Contains(out, "--commit") {
		t.Errorf("must name the flag that would do it:\n%s", out)
	}
	if n := run(t, filepath.Join(root, "api"), "rev-list", "--count", "--all"); n != "0" {
		t.Errorf("api has %s commits, want none", n)
	}
}
