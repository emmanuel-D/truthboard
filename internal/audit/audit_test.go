package audit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture builds a repo covering every inference path:
//   - feature/merged: true merge commit into main            → done
//   - feature/squashed: cherry-picked onto main (squash-like) → done
//   - feature/stalled: last commit 30 days ago, unmerged      → stalled
//   - feature/active: committed today, unmerged               → in-progress
//   - a direct commit on main                                 → shadow work
type fixture struct {
	t    *testing.T
	dir  string
	tick int
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, dir: t.TempDir()}
	f.git("init", "-b", "main")
	f.git("config", "user.email", "test@example.com")
	f.git("config", "user.name", "Test")
	f.git("config", "commit.gpgsign", "false")
	return f
}

func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", f.dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitAt runs a commit-creating git command (merge, cherry-pick) with the
// author/committer clock pinned to when.
func (f *fixture) gitAt(when time.Time, args ...string) {
	f.t.Helper()
	stamp := when.Format(time.RFC3339)
	cmd := exec.Command("git", append([]string{"-C", f.dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (f *fixture) commit(msg string, when time.Time) {
	f.t.Helper()
	f.tick++
	path := filepath.Join(f.dir, fmt.Sprintf("file%d.txt", f.tick))
	if err := os.WriteFile(path, []byte(msg), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.git("add", "-A")
	stamp := when.Format(time.RFC3339)
	cmd := exec.Command("git", "-C", f.dir, "commit", "-m", msg)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE="+stamp, "GIT_COMMITTER_DATE="+stamp)
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("commit %q: %v\n%s", msg, err, out)
	}
}

func buildStandardFixture(t *testing.T, now time.Time) *fixture {
	f := newFixture(t)
	old := now.AddDate(0, 0, -30)

	f.commit("chore: initial commit", old)

	f.git("checkout", "-b", "feature/merged")
	f.commit("feat: merged work", old)
	f.git("checkout", "main")
	f.gitAt(old, "merge", "--no-ff", "-m", "Merge branch 'feature/merged'", "feature/merged")

	f.git("checkout", "-b", "feature/squashed", "main")
	f.commit("feat: squashed work", now.AddDate(0, 0, -2))
	squashedTip := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	// One day later than the branch commit: identical dates would reproduce
	// the exact same SHA (same tree, parent, message), turning the branch
	// into a true ancestor and bypassing the patch-equivalence path.
	f.gitAt(now.AddDate(0, 0, -1), "cherry-pick", squashedTip)

	f.git("checkout", "-b", "feature/stalled", "main")
	f.commit("feat: stalled work", old)

	f.git("checkout", "-b", "feature/active", "main")
	f.commit("feat: active work", now.AddDate(0, 0, -1))

	f.git("checkout", "main")
	f.commit("fix: direct-to-main hotfix", now.AddDate(0, 0, -1))
	return f
}

func unitByName(t *testing.T, res *Result, name string) Unit {
	t.Helper()
	for _, u := range res.Units {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("unit %q not found in %+v", name, res.Units)
	return Unit{}
}

func TestAuditStatuses(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	if res.Integration != "main" {
		t.Errorf("integration = %q, want main", res.Integration)
	}
	for name, want := range map[string]Status{
		"feature/merged":   Done,
		"feature/squashed": Done,
		"feature/stalled":  Stalled,
		"feature/active":   InProgress,
	} {
		if got := unitByName(t, res, name).Status; got != want {
			t.Errorf("%s: status = %q, want %q (evidence: %s)", name, got, want, unitByName(t, res, name).Evidence)
		}
	}

	sq := unitByName(t, res, "feature/squashed")
	if !strings.Contains(sq.Evidence, "patch-equivalent") {
		t.Errorf("squashed evidence should cite patch-equivalence, got %q", sq.Evidence)
	}
}

func TestDrift(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Drift.StalePromises) != 1 || res.Drift.StalePromises[0].Name != "feature/stalled" {
		t.Errorf("stale promises = %+v, want exactly feature/stalled", res.Drift.StalePromises)
	}
	if len(res.Drift.LandedNotDeleted) != 2 {
		t.Errorf("landed-not-deleted = %+v, want feature/merged and feature/squashed", res.Drift.LandedNotDeleted)
	}

	// Shadow work must include the direct hotfix and the cherry-pick (both
	// landed on main outside a merge), but never the merge commit itself.
	var subjects []string
	for _, c := range res.Drift.ShadowWork {
		subjects = append(subjects, c.Subject)
	}
	joined := strings.Join(subjects, "\n")
	if !strings.Contains(joined, "direct-to-main hotfix") {
		t.Errorf("shadow work should include the hotfix, got:\n%s", joined)
	}
	if strings.Contains(joined, "Merge branch") {
		t.Errorf("shadow work must exclude merge commits, got:\n%s", joined)
	}
}

func TestShadowWorkExemptsIntentOnlyCommits(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -3))

	// A story written straight to main — backlog grooming, a shared-board
	// edit — is intent, not work that bypassed the branch flow.
	specDir := filepath.Join(f.dir, ".truthboard", "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSpec := func(name string) {
		if err := os.WriteFile(filepath.Join(specDir, name),
			[]byte("---\nid: "+name[:7]+"\ntitle: x\n---\n\n## Goal\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSpec("tb-aaaa-from-the-road.md")
	f.git("add", "-A")
	f.gitAt(now.AddDate(0, 0, -1), "commit", "-m", "Intent: from the road (tb-aaaa) — new story from the shared board")

	// Code smuggled into an intent commit is still shadow work.
	writeSpec("tb-bbbb-second.md")
	if err := os.WriteFile(filepath.Join(f.dir, "smuggled.go"), []byte("package x"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.git("add", "-A")
	f.gitAt(now.AddDate(0, 0, -1), "commit", "-m", "story plus smuggled code")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range res.Drift.ShadowWork {
		subjects = append(subjects, c.Subject)
	}
	joined := strings.Join(subjects, "\n")
	if strings.Contains(joined, "from the road") {
		t.Errorf("intent-only commits must not be shadow work, got:\n%s", joined)
	}
	if !strings.Contains(joined, "smuggled code") {
		t.Errorf("a commit mixing intent and code must stay shadow work, got:\n%s", joined)
	}
}

func TestDigestWindow(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)

	res, err := Audit(f.dir, Options{Now: now, DigestDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	// Within 14 days: cherry-picked squash + hotfix. The 30-day-old initial
	// commit and merge must be outside the window.
	if len(res.Digest) != 2 {
		t.Errorf("digest = %+v, want 2 commits within the window", res.Digest)
	}
}

func TestIntegrationElectionOverridesStaleOriginHEAD(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("chore: initial commit", now.AddDate(0, 0, -60))

	// Simulate a forge whose default branch pointer is stale: origin/HEAD
	// points at old-default, while main got all the recent activity.
	f.git("checkout", "-b", "old-default")
	f.commit("feat: ancient work", now.AddDate(0, 0, -50))
	f.git("checkout", "main")
	f.gitAt(now.AddDate(0, 0, -50), "merge", "--no-ff", "-m", "Merge branch 'old-default'", "old-default")
	f.commit("feat: recent work on main", now.AddDate(0, 0, -1))

	bare := t.TempDir()
	f.git("init", "--bare", bare)
	f.git("remote", "add", "origin", bare)
	f.git("push", "origin", "main", "old-default")
	f.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/old-default")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if res.Integration != "origin/main" {
		t.Errorf("integration = %q, want origin/main despite origin/HEAD", res.Integration)
	}
	if res.ElectionNote == "" || !strings.Contains(res.ElectionNote, "old-default") {
		t.Errorf("expected a misconfiguration note naming old-default, got %q", res.ElectionNote)
	}
}

func TestRemoteOnlyBranchesAreSeen(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)

	bare := t.TempDir()
	f.git("init", "--bare", bare)
	f.git("remote", "add", "origin", bare)
	f.git("push", "origin", "--all")
	// Delete the local feature branches; only origin/* copies remain.
	f.git("checkout", "main")
	for _, b := range []string{"feature/merged", "feature/squashed", "feature/stalled", "feature/active"} {
		f.git("branch", "-D", b)
	}

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := unitByName(t, res, "feature/active").Status; got != InProgress {
		t.Errorf("remote-only feature/active: status = %q, want in-progress", got)
	}
	if len(res.Units) != 4 {
		t.Errorf("units = %d, want 4 (remote-only branches must still be audited)", len(res.Units))
	}
}

// The adoption commit writes the whole governed fileset at once and lands
// directly on the integration branch — there is no board to open an MR
// against yet. Flagging it made every adopter's first board accuse their
// own setup of drift.
func TestAdoptionCommitIsNotShadowWork(t *testing.T) {
	now := time.Now()
	f := newFixture(t)
	f.commit("Initial commit", now.AddDate(0, 0, -3))

	if err := os.MkdirAll(filepath.Join(f.dir, ".truthboard", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		".mcp.json": `{"mcpServers":{}}`,
		"AGENTS.md": "# agreement\n",
		"CLAUDE.md": "@AGENTS.md\n",
		".truthboard/specs/tb-cccc-first.md": "---\nid: tb-cccc\ntitle: x\n---\n\n## Goal\n",
	} {
		if err := os.WriteFile(filepath.Join(f.dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f.git("add", "-A")
	f.gitAt(now.AddDate(0, 0, -1), "commit", "-m", "Track work with truthboard")

	res, err := Audit(f.dir, Options{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range res.Drift.ShadowWork {
		if strings.Contains(c.Subject, "Track work with truthboard") {
			t.Errorf("the adoption commit must not be shadow work, got: %s", c.Subject)
		}
	}
}
