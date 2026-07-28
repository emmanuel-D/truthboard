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

// countingGit puts a shim ahead of git on PATH so a test can count how many
// git processes an audit spawns. Wall time on a small fixture says nothing
// useful; the process count is what grew quadratically and what has to stay
// flat.
func countingGit(t *testing.T) func() int {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on PATH")
	}
	shim := fmt.Sprintf("#!/bin/sh\necho call >> %q\nexec %q \"$@\"\n", log, real)
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return func() int {
		raw, err := os.ReadFile(log)
		if os.IsNotExist(err) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return len(strings.Fields(string(raw)))
	}
}

// TestAuditCostDoesNotGrowWithSpecCount is the regression guard for the
// fan-out that made truthboard's own board unusable: linking asked git
// "does branch B carry spec S?" once per pair, so 68 specs across 17 refs
// spawned 940 `git log --grep` processes and a single audit took fifteen
// seconds — with the board's cache mutex held throughout, which is what
// viewers saw as "audit unavailable — retrying".
//
// Specs are intent and cost nothing to add, so their number must not
// multiply the work. Adding specs to an unchanged repo must not add git
// processes at all.
func TestAuditCostDoesNotGrowWithSpecCount(t *testing.T) {
	f := newFixture(t)
	now := time.Now()
	f.commit("chore: init", now)

	// Enough branches that a per-pair implementation multiplies visibly.
	for i := 0; i < 6; i++ {
		branch := fmt.Sprintf("feature/work-%d", i)
		f.git("checkout", "-q", "-b", branch)
		f.commit(fmt.Sprintf("feat: %d\n\nSpec: tb-br%02d", i, i), now)
		f.git("checkout", "-q", "main")
	}

	countFew := auditCalls(t, f.dir, 3)
	countMany := auditCalls(t, f.dir, 40)

	t.Logf("git processes: %d specs -> %d calls; %d specs -> %d calls", 3, countFew, 40, countMany)
	if countMany != countFew {
		t.Errorf("adding specs added %d git processes (%d -> %d) — linking is asking git per (spec × branch) again, "+
			"which is what made a 68-spec board take fifteen seconds per audit",
			countMany-countFew, countFew, countMany)
	}
}

// auditCalls writes n specs into repo, runs one audit, and returns how many
// git processes it took.
func auditCalls(t *testing.T, repo string, n int) int {
	t.Helper()
	specs := filepath.Join(repo, ".truthboard", "specs")
	if err := os.RemoveAll(specs); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		writeSpec(t, repo, fmt.Sprintf("tb-s%03d", i), fmt.Sprintf("Story %d", i), "")
	}
	calls := countingGit(t)
	if _, err := Audit(repo, Options{}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	return calls()
}
