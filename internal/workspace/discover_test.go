package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// siblings builds a workspace folder: a hub plus a git checkout per named
// repo, each with an origin unless its name is listed in noOrigin.
func siblings(t *testing.T, noOrigin map[string]bool, names ...string) string {
	t.Helper()
	root := t.TempDir()
	hub := filepath.Join(root, "hub")
	if err := os.MkdirAll(hub, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", hub, "init", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init hub: %v\n%s", err, out)
	}
	for _, name := range names {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("git", "-C", dir, "init", "-b", "main").CombinedOutput(); err != nil {
			t.Fatalf("git init %s: %v\n%s", name, err, out)
		}
		if noOrigin[name] {
			continue
		}
		url := "git@example.com:acme/" + name + ".git"
		if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", url).CombinedOutput(); err != nil {
			t.Fatalf("git remote add %s: %v\n%s", name, err, out)
		}
	}
	return hub
}

func discover(t *testing.T, hub string) []Candidate {
	t.Helper()
	declared, err := Load(hub)
	if err != nil {
		t.Fatal(err)
	}
	found, err := Discover(hub, declared)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestDiscoveryReadsWhatWouldOtherwiseBeTyped(t *testing.T) {
	hub := siblings(t, nil, "api", "web")
	found := discover(t, hub)
	if len(found) != 2 {
		t.Fatalf("want api and web, got %+v", found)
	}
	for i, want := range []Candidate{
		{Name: "api", Path: "../api", Remote: "git@example.com:acme/api.git"},
		{Name: "web", Path: "../web", Remote: "git@example.com:acme/web.git"},
	} {
		if found[i] != want {
			t.Errorf("candidate %d = %+v, want %+v", i, found[i], want)
		}
	}
}

// A workspace folder holds plenty that is no spoke.
func TestOnlyGitWorkTreesAreOffered(t *testing.T) {
	hub := siblings(t, nil, "api")
	root := filepath.Dir(hub)
	if err := os.MkdirAll(filepath.Join(root, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pitch.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := discover(t, hub)
	if len(found) != 1 || found[0].Name != "api" {
		t.Errorf("plain directories and files are not repositories, got %+v", found)
	}
}

func TestTheHubIsNeverOfferedAsItsOwnSpoke(t *testing.T) {
	hub := siblings(t, nil, "api")
	for _, c := range discover(t, hub) {
		if c.Name == "hub" || c.Path == "." {
			t.Errorf("the hub proposed as a spoke: %+v", c)
		}
	}
}

// A checkout with no readable origin is still worth watching locally, and
// skipping it in silence is how a repo ends up watched by nobody.
func TestARepoWithNoOriginIsOfferedAsPathOnly(t *testing.T) {
	hub := siblings(t, map[string]bool{"infra": true}, "api", "infra")
	found := discover(t, hub)
	if len(found) != 2 {
		t.Fatalf("want both, got %+v", found)
	}
	var infra Candidate
	for _, c := range found {
		if c.Name == "infra" {
			infra = c
		}
	}
	if infra.Path != "../infra" || infra.Remote != "" {
		t.Errorf("infra = %+v, want a path-only candidate", infra)
	}
	// Declarable as it stands: a path alone is a valid spoke.
	if _, err := Declare(hub, Repos([]Candidate{infra})); err != nil {
		t.Errorf("a path-only candidate must be declarable: %v", err)
	}
}

func TestAlreadyDeclaredReposAreNotOfferedAgain(t *testing.T) {
	hub := siblings(t, nil, "api", "web")
	if _, err := Declare(hub, []Repo{{Name: "api", Remote: "git@example.com:acme/api.git", Path: "../api"}}); err != nil {
		t.Fatal(err)
	}
	found := discover(t, hub)
	if len(found) != 1 || found[0].Name != "web" {
		t.Errorf("a re-run must offer only what is undeclared, got %+v", found)
	}
}

// Discovery is read-only: reading a config file is not cloning, and nothing
// it looks at may be fetched or written.
func TestDiscoveryNeverWrites(t *testing.T) {
	hub := siblings(t, nil, "api")
	api := filepath.Join(filepath.Dir(hub), "api")
	before, err := exec.Command("git", "-C", api, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	discover(t, hub)
	after, err := exec.Command("git", "-C", api, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("discovery changed the repo: %q -> %q", before, after)
	}
	if _, err := os.Stat(filepath.Join(hub, ".truthboard", "workspace.yml")); err == nil {
		t.Error("discovery must declare nothing on its own")
	}
}

func TestDerivedNamesAreValidSpokeNames(t *testing.T) {
	hub := siblings(t, nil, "LetTalk_Server", "web v2")
	for _, c := range discover(t, hub) {
		if err := ValidName(c.Name); err != nil {
			t.Errorf("derived name %q is not declarable: %v", c.Name, err)
		}
	}
}
