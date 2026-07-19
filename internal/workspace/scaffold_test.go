package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func manifest(t *testing.T, hub string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(hub, filepath.FromSlash(File)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestDeclareScaffoldsFreshManifest: name=remote and path-only spokes land
// in a manifest the loader accepts, reported per spoke.
func TestDeclareScaffoldsFreshManifest(t *testing.T) {
	hub := t.TempDir()
	log, err := Declare(hub, []Repo{
		{Name: "api", Remote: "git@example.com:acme/api.git"},
		{Name: "infra", Path: "../infra"},
		{Name: "web", Remote: "git@example.com:acme/web.git", Path: "../web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 3 {
		t.Fatalf("log = %v, want one line per spoke", log)
	}

	ws, err := Load(hub)
	if err != nil {
		t.Fatalf("scaffolded manifest must load: %v", err)
	}
	if len(ws.Repos) != 3 || ws.Repos[0].Name != "api" || ws.Repos[1].Name != "infra" || ws.Repos[2].Name != "web" {
		t.Fatalf("roundtrip repos = %+v", ws.Repos)
	}
	if ws.Repos[0].Remote != "git@example.com:acme/api.git" || ws.Repos[1].Path != "../infra" ||
		ws.Repos[2].Remote == "" || ws.Repos[2].Path != "../web" {
		t.Fatalf("roundtrip fields = %+v", ws.Repos)
	}
}

// TestDeclareValidatesBeforeWriting: any invalid declaration in the batch
// means nothing is written at all.
func TestDeclareValidatesBeforeWriting(t *testing.T) {
	cases := []struct {
		name  string
		repos []Repo
		want  string
	}{
		{"bad grammar", []Repo{{Name: "API", Remote: "x"}}, "lowercase"},
		{"reserved hub", []Repo{{Name: "hub", Remote: "x"}}, "reserved"},
		{"duplicate", []Repo{{Name: "api", Remote: "x"}, {Name: "api", Remote: "y"}}, "twice"},
		{"neither remote nor path", []Repo{{Name: "api"}}, "needs a remote"},
	}
	for _, c := range cases {
		hub := t.TempDir()
		_, err := Declare(hub, append([]Repo{{Name: "ok", Remote: "git@example.com:a/ok.git"}}, c.repos...))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mention of %q", c.name, err, c.want)
		}
		if _, statErr := os.Stat(filepath.Join(hub, filepath.FromSlash(File))); !os.IsNotExist(statErr) {
			t.Errorf("%s: manifest written despite invalid batch", c.name)
		}
	}

	if _, err := Declare(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "nothing to declare") {
		t.Errorf("empty batch: err = %v, want nothing-to-declare", err)
	}
}

// TestDeclareMergesWithoutRewriting is the re-run promise: new spokes join
// the manifest, existing ones — including hand-written comments and fields
// the scaffold cannot set — survive byte-for-byte semantics intact.
func TestDeclareMergesWithoutRewriting(t *testing.T) {
	hub := t.TempDir()
	write(t, hub, "# reviewed by the team 2026-07\nrepos:\n  api:\n    remote: git@example.com:acme/api.git\n    integration: develop\n")

	log, err := Declare(hub, []Repo{
		{Name: "api", Remote: "git@example.com:acme/api.git"}, // identical redeclare
		{Name: "web", Remote: "git@example.com:acme/web.git"},
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "api already declared") || !strings.Contains(joined, "declared spoke web") {
		t.Fatalf("log = %v", log)
	}

	raw := manifest(t, hub)
	if !strings.Contains(raw, "# reviewed by the team 2026-07") {
		t.Errorf("merge lost the hand-written comment:\n%s", raw)
	}
	ws, err := Load(hub)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("repos = %+v", ws.Repos)
	}
	if ws.Repos[0].Integration != "develop" {
		t.Errorf("existing entry's integration was rewritten: %+v", ws.Repos[0])
	}
}

// TestDeclareRefusesToRewrite: same name, different remote — loud error,
// manifest untouched.
func TestDeclareRefusesToRewrite(t *testing.T) {
	hub := t.TempDir()
	write(t, hub, "repos:\n  api:\n    remote: git@example.com:acme/api.git\n")
	before := manifest(t, hub)

	_, err := Declare(hub, []Repo{{Name: "api", Remote: "git@example.com:other/api.git"}})
	if err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("err = %v, want already-declared refusal", err)
	}
	if manifest(t, hub) != before {
		t.Error("manifest changed despite refusal")
	}
}

// TestDeclareRefusesMalformedManifest: a manifest the loader rejects must
// never be appended to.
func TestDeclareRefusesMalformedManifest(t *testing.T) {
	hub := t.TempDir()
	write(t, hub, "repos:\n  api: {}\n")
	before := manifest(t, hub)

	_, err := Declare(hub, []Repo{{Name: "web", Remote: "git@example.com:acme/web.git"}})
	if err == nil {
		t.Fatal("want error appending to a malformed manifest")
	}
	if manifest(t, hub) != before {
		t.Error("malformed manifest was modified")
	}
}
