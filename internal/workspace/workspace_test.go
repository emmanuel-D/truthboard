package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, hub, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(hub, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hub, filepath.FromSlash(File)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingManifestMeansSingleRepo(t *testing.T) {
	ws, err := Load(t.TempDir())
	if err != nil || ws != nil {
		t.Fatalf("missing manifest should be (nil, nil), got %v, %v", ws, err)
	}
}

func TestLoadParsesAndSorts(t *testing.T) {
	hub := t.TempDir()
	write(t, hub, `
repos:
  web:
    remote: git@example.com:acme/web.git
  api:
    remote: git@example.com:acme/api.git
    integration: main
`)
	ws, err := Load(hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 || ws.Repos[0].Name != "api" || ws.Repos[1].Name != "web" {
		t.Fatalf("expected sorted [api web], got %+v", ws.Repos)
	}
	if ws.Repos[0].Integration != "main" {
		t.Fatalf("integration not parsed: %+v", ws.Repos[0])
	}
}

func TestLoadRejectsBadManifests(t *testing.T) {
	cases := map[string]string{
		"empty repos":       "repos: {}\n",
		"bad name":          "repos:\n  \"api:v2\":\n    remote: x\n",
		"no remote no path": "repos:\n  api: {}\n",
		"unknown field":     "repos:\n  api:\n    remote: x\n    status: done\n",
	}
	for name, content := range cases {
		hub := t.TempDir()
		write(t, hub, content)
		if _, err := Load(hub); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

func TestResolvePrefersDeclaredPath(t *testing.T) {
	hub := t.TempDir()
	spoke := filepath.Join(hub, "..", filepath.Base(hub)+"-spoke")
	if err := os.MkdirAll(spoke, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(spoke) })

	ws := &Workspace{Hub: hub}
	got, err := ws.Resolve(Repo{Name: "api", Path: "../" + filepath.Base(spoke)})
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.Abs(spoke); mustAbs(t, got) != want {
		t.Fatalf("resolved %s, want %s", got, spoke)
	}
}

func TestResolveUnreachableIsLoud(t *testing.T) {
	ws := &Workspace{Hub: t.TempDir()}
	_, err := ws.Resolve(Repo{Name: "api", Remote: "git@example.com:acme/api.git"})
	if err == nil || !strings.Contains(err.Error(), "no local copy") {
		t.Fatalf("expected a 'no local copy' error, got %v", err)
	}
	_, err = ws.Resolve(Repo{Name: "web", Path: "does/not/exist"})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a missing-path error, got %v", err)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestSameRemoteAcrossURLForms(t *testing.T) {
	// Every form below names the same repository. Missing any of these would
	// make the check cry wolf on a correct setup.
	equal := []string{
		"git@github.com:acme/api.git",
		"git@github.com:acme/api",
		"https://github.com/acme/api.git",
		"https://github.com/acme/api",
		"https://github.com/acme/api/",
		"https://user:ghp_secret@github.com/acme/api.git",
		"ssh://git@github.com/acme/api.git",
		"https://GitHub.com/Acme/API.git",
	}
	for _, a := range equal {
		for _, b := range equal {
			if !sameRemote(a, b) {
				t.Errorf("sameRemote(%q, %q) = false, want true (%q vs %q)",
					a, b, normalizeRemote(a), normalizeRemote(b))
			}
		}
	}

	different := [][2]string{
		{"https://github.com/acme/api.git", "https://github.com/acme/web.git"},
		{"https://github.com/acme/api.git", "https://gitlab.com/acme/api.git"},
		{"https://github.com/acme/api.git", "https://github.com/other/api.git"},
	}
	for _, pair := range different {
		if sameRemote(pair[0], pair[1]) {
			t.Errorf("sameRemote(%q, %q) = true, want false", pair[0], pair[1])
		}
	}
}

// spokeAt makes a checkout at dir whose origin is remote ("" for no origin).
func spokeAt(t *testing.T, dir, remote string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if remote != "" {
		run("remote", "add", "origin", remote)
	}
}

func TestResolveRefusesACheckoutOfAnotherRepo(t *testing.T) {
	hub := t.TempDir()
	spoke := filepath.Join(hub, "wrong")
	spokeAt(t, spoke, "https://github.com/acme/web.git")

	w := &Workspace{Hub: hub}
	_, err := w.Resolve(Repo{Name: "api", Remote: "https://github.com/acme/api.git", Path: "wrong"})
	if err == nil {
		t.Fatal("Resolve() accepted a checkout of a different repo")
	}
	// Both URLs must be named — "wrong repo" alone does not tell you which.
	for _, want := range []string{"acme/web", "acme/api", "wrong"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

func TestResolveAcceptsEquivalentRemoteForms(t *testing.T) {
	hub := t.TempDir()
	spoke := filepath.Join(hub, "api")
	spokeAt(t, spoke, "git@github.com:acme/api.git")

	w := &Workspace{Hub: hub}
	got, err := w.Resolve(Repo{Name: "api", Remote: "https://github.com/acme/api", Path: "api"})
	if err != nil {
		t.Fatalf("equivalent remote forms must resolve: %v", err)
	}
	if got != spoke {
		t.Errorf("Resolve() = %q, want %q", got, spoke)
	}
}

// Nothing to compare against must never become a refusal: these are valid
// setups, and a check that fires on them gets switched off.
func TestResolveAllowsWhatItCannotDisprove(t *testing.T) {
	hub := t.TempDir()
	noOrigin := filepath.Join(hub, "local")
	spokeAt(t, noOrigin, "")
	pathOnly := filepath.Join(hub, "pathonly")
	spokeAt(t, pathOnly, "https://github.com/acme/anything.git")

	w := &Workspace{Hub: hub}
	if _, err := w.Resolve(Repo{Name: "local", Remote: "https://github.com/acme/api.git", Path: "local"}); err != nil {
		t.Errorf("a checkout with no origin proves no mismatch: %v", err)
	}
	if _, err := w.Resolve(Repo{Name: "pathonly", Path: "pathonly"}); err != nil {
		t.Errorf("a path-only spoke has nothing to verify: %v", err)
	}
}

// The pre-existing message for a spoke that simply is not there must survive.
func TestResolveKeepsNoLocalCopyMessage(t *testing.T) {
	hub := t.TempDir()
	w := &Workspace{Hub: hub}
	_, err := w.Resolve(Repo{Name: "api", Remote: "https://github.com/acme/api.git", Path: "missing"})
	if err == nil || !strings.Contains(err.Error(), "no local copy") {
		t.Errorf("err = %v, want the no-local-copy message", err)
	}
}
