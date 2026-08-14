package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emmanuel-D/truthboard/internal/spec"
)

func writeSpec(t *testing.T, repo, id, title, branch string) {
	t.Helper()
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nid: " + id + "\ntitle: " + title + "\n"
	if branch != "" {
		content += "branch: " + branch + "\n"
	}
	content += "---\n\n## Goal\nTest.\n"
	if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func specByID(t *testing.T, res *Result, id string) SpecStatus {
	t.Helper()
	for _, s := range res.Specs {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("spec %q not found in %+v", id, res.Specs)
	return SpecStatus{}
}

func TestSpecStatusDerivation(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	old := now.AddDate(0, 0, -30)

	f.commit("chore: initial commit", old)

	// tb-aaaa: id appears in the branch name, branch is active.
	f.git("checkout", "-b", "feature/tb-aaaa-login")
	f.commit("feat: login work", now.AddDate(0, 0, -1))

	// tb-bbbb: linked only by commit trailer on an unrelated branch name.
	f.git("checkout", "-b", "wip/something", "main")
	f.commit("feat: mystery work\n\nSpec: tb-bbbb", now.AddDate(0, 0, -1))

	// tb-cccc: trailer commit merged into main, no live branch.
	f.git("checkout", "-b", "feature/done-work", "main")
	f.commit("feat: finished work\n\nSpec: tb-cccc", now.AddDate(0, 0, -3))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -3), "merge", "--no-ff", "-m", "Merge branch 'feature/done-work'", "feature/done-work")
	f.git("branch", "-D", "feature/done-work")

	// tb-dddd: spec exists, nothing in git yet.
	writeSpec(t, f.dir, "tb-aaaa", "Login flow", "")
	writeSpec(t, f.dir, "tb-bbbb", "Trailer-linked work", "")
	writeSpec(t, f.dir, "tb-cccc", "Finished work", "")
	writeSpec(t, f.dir, "tb-dddd", "Future work", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]Status{
		"tb-aaaa": InProgress,
		"tb-bbbb": InProgress,
		"tb-cccc": Done,
		"tb-dddd": Planned,
	} {
		got := specByID(t, res, id)
		if got.Status != want {
			t.Errorf("%s: status = %q, want %q (evidence: %s)", id, got.Status, want, got.Evidence)
		}
	}

	if got := specByID(t, res, "tb-bbbb"); len(got.Branches) != 1 || got.Branches[0] != "wip/something" {
		t.Errorf("tb-bbbb branches = %v, want [wip/something] via trailer", got.Branches)
	}
	if got := unitByName(t, res, "feature/tb-aaaa-login"); got.SpecID != "tb-aaaa" {
		t.Errorf("unit spec link = %q, want tb-aaaa", got.SpecID)
	}
}

func TestEmptyBranchIsNotAUnit(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	// Freshly cut branch, zero commits of its own: must not appear on the
	// board, and its spec must stay planned rather than reading as done.
	f.git("branch", "feature/tb-ffff-fresh")
	writeSpec(t, f.dir, "tb-ffff", "Fresh work", "")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Units) != 0 {
		t.Errorf("units = %+v, want none (empty branch is not a unit)", res.Units)
	}
	if got := specByID(t, res, "tb-ffff"); got.Status != Planned {
		t.Errorf("spec on empty branch: status = %q, want planned (evidence: %s)", got.Status, got.Evidence)
	}
}

func TestSpecBranchGlobLinking(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	f.git("checkout", "-b", "feature/custom-name")
	f.commit("feat: glob-linked work", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	writeSpec(t, f.dir, "tb-eeee", "Glob-linked", "feature/custom-*")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	got := specByID(t, res, "tb-eeee")
	if got.Status != InProgress || len(got.Branches) != 1 {
		t.Errorf("glob spec = %+v, want in-progress via feature/custom-name", got)
	}
}

func TestSpecBranchGlobCrossesSlashesWithDoublestar(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))
	f.git("checkout", "-b", "feat/nested/custom-name")
	f.commit("feat: deep work, no id anywhere", now.AddDate(0, 0, -1))
	f.git("checkout", "main")

	// Same dialect as spec paths: ** crosses slashes; plain * would not.
	writeSpec(t, f.dir, "tb-gggg", "Deep glob", "feat/**")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := specByID(t, res, "tb-gggg"); got.Status != InProgress || len(got.Branches) != 1 {
		t.Errorf("doublestar glob = %+v, want linked to feat/nested/custom-name", got)
	}
}

func TestBacklogOrderingAndFields(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -30))

	dir := spec.Dir(f.dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(id, prio, epic string) {
		content := "---\nid: " + id + "\ntitle: Story " + id + "\n"
		if prio != "" {
			content += "priority: " + prio + "\n"
		}
		if epic != "" {
			content += "epic: " + epic + "\n"
		}
		content += "---\n\n## Goal\nTest.\n"
		if err := os.WriteFile(filepath.Join(dir, id+"-test.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("tb-aa01", "2", "epic-b")
	write("tb-bb02", "1", "epic-a")
	write("tb-cc03", "", "") // unset priority sorts last

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, s := range res.Specs {
		order = append(order, s.ID)
	}
	want := []string{"tb-bb02", "tb-aa01", "tb-cc03"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("backlog order = %v, want %v (priority first, unset last)", order, want)
		}
	}
	if s := specByID(t, res, "tb-bb02"); s.Epic != "epic-a" || s.Priority != 1 {
		t.Errorf("spec status must carry epic/priority, got %+v", s)
	}
}

func TestSpecRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := spec.New(dir, "Add email verification to signup", "emmanuel")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(s.ID, "tb-") {
		t.Errorf("id = %q, want tb- prefix", s.ID)
	}

	loaded, err := spec.Find(dir, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != s.Title || loaded.Owner != "emmanuel" || !strings.Contains(loaded.Body, "## Goal") {
		t.Errorf("round trip lost data: %+v", loaded)
	}

	loaded.Branch = "hotfix/*"
	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	again, err := spec.Find(dir, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Branch != "hotfix/*" {
		t.Errorf("branch after save = %q, want hotfix/*", again.Branch)
	}
}

// commitContents writes each path=content pair and commits them with msg at
// when — the intent/implementation distinction is about *which files* a
// commit touches, which the shared fixture's one-file-per-commit helper
// cannot express.
func (f *fixture) commitContents(msg string, when time.Time, files map[string]string) {
	f.t.Helper()
	for rel, content := range files {
		path := filepath.Join(f.dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			f.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	f.git("add", "-A")
	f.gitAt(when, "commit", "-m", msg)
}

func specFile(id string) string {
	return ".truthboard/specs/" + id + "-test.md"
}

func specBody(id, title string) string {
	return "---\nid: " + id + "\ntitle: " + title + "\n---\n\n## Goal\nTest.\n\n## Acceptance\n\n- [ ] unticked on purpose\n"
}

// TestFilingAStoryIsNotLandingIt covers the defect where the commit that
// creates a spec file — which necessarily carries that spec's own trailer —
// was elected as the story's landing commit, deriving done for work nobody
// had started.
func TestFilingAStoryIsNotLandingIt(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))

	// tb-aaaa: filed straight to main, the documented way to add intent.
	f.commitContents("Story: something worth doing\n\nSpec: tb-aaaa", now.AddDate(0, 0, -5),
		map[string]string{specFile("tb-aaaa"): specBody("tb-aaaa", "Filed, not built")})

	// tb-bbbb: filed and delivered in one commit — the spec edit rides along
	// with the implementation, which is how most stories actually land.
	f.commitContents("feat: build it\n\nSpec: tb-bbbb", now.AddDate(0, 0, -4),
		map[string]string{
			specFile("tb-bbbb"): specBody("tb-bbbb", "Filed and built"),
			"pkg/thing.go":      "package pkg",
		})

	// tb-cccc: filed first, delivered later. The landing commit must be the
	// implementation, not the filing — CI checks are read against this SHA.
	f.commitContents("Story: do it later\n\nSpec: tb-cccc", now.AddDate(0, 0, -3),
		map[string]string{specFile("tb-cccc"): specBody("tb-cccc", "Filed then built")})
	f.commitContents("feat: did it\n\nSpec: tb-cccc", now.AddDate(0, 0, -2),
		map[string]string{"pkg/later.go": "package pkg"})
	implSHA := f.git("rev-parse", "HEAD")

	// An intent commit landing *after* the implementation must not unland it.
	f.commitContents("Story: tick the boxes\n\nSpec: tb-cccc", now.AddDate(0, 0, -1),
		map[string]string{specFile("tb-cccc"): specBody("tb-cccc", "Filed then built") + "\n- [x] done\n"})

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	if got := specByID(t, res, "tb-aaaa"); got.Status != Planned {
		t.Errorf("a story that was only filed = %q (%s), want planned", got.Status, got.Evidence)
	}
	// Acceptance boxes are never load-bearing: this spec has none ticked and
	// must still derive done from the code that landed.
	if got := specByID(t, res, "tb-bbbb"); got.Status != Done {
		t.Errorf("a story filed and built in one commit = %q (%s), want done", got.Status, got.Evidence)
	} else if got.AcceptanceDone != 0 {
		t.Fatalf("fixture drift: tb-bbbb should have no ticked boxes, got %d", got.AcceptanceDone)
	}
	got := specByID(t, res, "tb-cccc")
	if got.Status != Done {
		t.Errorf("a story filed then built = %q (%s), want done", got.Status, got.Evidence)
	}
	if got.Landed != implSHA {
		t.Errorf("landing commit = %s, want the implementation %s", short(got.Landed), short(implSHA))
	}

	// The digest headline must not announce a story nobody has built.
	for _, s := range res.Shipped {
		if s.ID == "tb-aaaa" {
			t.Errorf("a filed-only story reached the digest as shipped: %+v", s)
		}
	}
}

// TestNextHandsOutAFiledStory is the harm this defect did to the agent
// loop: a story filed on the integration branch derived done, so the one
// call an idle agent makes would never offer it.
func TestNextHandsOutAFiledStory(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -10))
	f.commitContents("Story: the only story\n\nSpec: tb-aaaa", now.AddDate(0, 0, -5),
		map[string]string{specFile("tb-aaaa"): specBody("tb-aaaa", "Filed, not built")})

	up, err := Next(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if up.Spec == nil || up.Spec.ID != "tb-aaaa" {
		t.Fatalf("next = %+v, want tb-aaaa", up.Spec)
	}
}

func TestIntentOnly(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"spec file alone", []string{".truthboard/specs/tb-aaaa-x.md"}, true},
		{"spec plus wiring", []string{".truthboard/specs/tb-aaaa-x.md", "AGENTS.md", ".vscode/mcp.json"}, true},
		{"spec plus code", []string{".truthboard/specs/tb-aaaa-x.md", "main.go"}, false},
		{"code alone", []string{"main.go"}, false},
		{"blank lines only", []string{"", "  "}, false},
		{"no files at all", nil, false},
		{"trailing blank line", []string{"AGENTS.md", ""}, true},
	}
	for _, tc := range cases {
		if got := intentOnly(tc.files); got != tc.want {
			t.Errorf("%s: intentOnly(%q) = %v, want %v", tc.name, tc.files, got, tc.want)
		}
	}
}
