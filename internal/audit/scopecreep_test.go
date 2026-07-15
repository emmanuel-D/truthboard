package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpecWithPaths(t *testing.T, repo, id, title string, paths []string) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\npaths: [" + strings.Join(paths, ", ") + "]\n---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) commitFiles(msg string, when time.Time, files ...string) {
	f.t.Helper()
	for _, name := range files {
		full := filepath.Join(f.dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(msg), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	// Add only the named files — add -A would sweep the untracked spec file
	// into the branch commit and lose it when the fixture checks out main.
	f.git(append([]string{"add"}, files...)...)
	f.gitAt(when, "commit", "-m", msg)
}

func TestScopeCreepFlagged(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	writeSpecWithPaths(t, f.dir, "tb-sc01", "Scoped work", []string{"app/**"})

	f.git("checkout", "-b", "feature/tb-sc01-work")
	f.commitFiles("feat: creeping work\n\nSpec: tb-sc01", now.AddDate(0, 0, -1),
		"app/inside.go", "lib/outside1.go", "lib/outside2.go", "docs/outside3.md")
	f.git("checkout", "main")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.ScopeCreep) != 1 {
		t.Fatalf("scope creep = %+v, want exactly one finding", res.Drift.ScopeCreep)
	}
	sc := res.Drift.ScopeCreep[0]
	if sc.SpecID != "tb-sc01" || sc.Branch != "feature/tb-sc01-work" || sc.Outside != 3 || sc.Total != 4 {
		t.Errorf("finding = %+v, want 3/4 outside on tb-sc01/feature/tb-sc01-work", sc)
	}
	if !strings.HasPrefix(sc.TopDirs, "lib (2)") {
		t.Errorf("top dirs = %q, want lib (2) first (directories, not files)", sc.TopDirs)
	}
}

func TestScopeCreepRequiresMajorityOutside(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	writeSpecWithPaths(t, f.dir, "tb-sc02", "Mostly in scope", []string{"app/**"})

	f.git("checkout", "-b", "feature/tb-sc02-work")
	f.commitFiles("feat: focused work\n\nSpec: tb-sc02", now.AddDate(0, 0, -1),
		"app/a.go", "app/b.go", "lib/one-stray.go")
	f.git("checkout", "main")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.ScopeCreep) != 0 {
		t.Errorf("scope creep = %+v, want none (1/3 outside is not creep)", res.Drift.ScopeCreep)
	}
}

func TestSpecsWithoutPathsNeverFlagged(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	writeSpec(t, f.dir, "tb-sc03", "No declared scope", "")

	f.git("checkout", "-b", "feature/tb-sc03-work")
	f.commitFiles("feat: anywhere\n\nSpec: tb-sc03", now.AddDate(0, 0, -1),
		"lib/x.go", "docs/y.md")
	f.git("checkout", "main")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Drift.ScopeCreep) != 0 {
		t.Errorf("scope creep = %+v, want none (paths are opt-in)", res.Drift.ScopeCreep)
	}
}

func TestMatchScopeDialect(t *testing.T) {
	for _, tc := range []struct {
		pattern, file string
		want          bool
	}{
		{"internal/audit/**", "internal/audit/specs.go", true},
		{"internal/audit/**", "internal/audit/sub/deep.go", true},
		{"internal/audit/**", "internal/report/report.go", false},
		{"internal/audit", "internal/audit/specs.go", true}, // bare dir = prefix
		{"internal/audit", "internal/auditor/x.go", false},
		{"*.md", "README.md", true},
		{"*.md", "docs/README.md", false}, // single * stays in one level
		{"cmd/*/main.go", "cmd/truthboard/main.go", true},
	} {
		if got := matchScope(tc.pattern, tc.file); got != tc.want {
			t.Errorf("matchScope(%q, %q) = %v, want %v", tc.pattern, tc.file, got, tc.want)
		}
	}
}
