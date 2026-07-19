package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRepos(t *testing.T) {
	hub := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hub, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}

	// No manifest: any repos: declaration is refused.
	if err := ValidateRepos(hub, []string{"api"}); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("repos without a manifest must fail loudly, got %v", err)
	}
	if err := ValidateRepos(hub, nil); err != nil {
		t.Fatalf("empty repos list is always fine, got %v", err)
	}

	manifest := "repos:\n  api:\n    remote: git@example.com:acme/api.git\n"
	if err := os.WriteFile(filepath.Join(hub, ".truthboard", "workspace.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ValidateRepos(hub, []string{"hub", "api"}); err != nil {
		t.Fatalf("hub + declared spoke must validate, got %v", err)
	}
	if err := ValidateRepos(hub, []string{"mobile"}); err == nil || !strings.Contains(err.Error(), "known repos: hub, api") {
		t.Fatalf("unknown repo must fail listing known ones, got %v", err)
	}
	if err := ValidateRepos(hub, []string{"api", "api"}); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("duplicates must fail, got %v", err)
	}
}

func TestReposRoundTripsThroughSave(t *testing.T) {
	dir := t.TempDir()
	s := &Spec{ID: "tb-1111", Title: "T", Repos: []string{"hub", "api"},
		File: filepath.Join(dir, "tb-1111-t.md")}
	s.Body = "## Goal\nX."
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := parseFile(s.File)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != 2 || got.Repos[0] != "hub" || got.Repos[1] != "api" {
		t.Fatalf("repos did not round-trip: %+v", got.Repos)
	}
}
