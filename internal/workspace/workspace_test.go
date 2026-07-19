package workspace

import (
	"os"
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
