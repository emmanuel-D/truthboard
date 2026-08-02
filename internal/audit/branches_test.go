package audit

import (
	"strings"
	"testing"
	"time"
)

// TestFindBranchReportsWhatMakesDeletionSafe covers the facts the board
// server acts on: whether the work is already in the integration branch,
// and the two conditions under which deleting is not a judgement call but
// impossible.
func TestFindBranchReportsWhatMakesDeletionSafe(t *testing.T) {
	now := time.Now()
	f := buildStandardFixture(t, now)
	f.git("checkout", "feature/active") // the repo is standing on this one

	for _, tc := range []struct {
		branch      string
		merged      bool
		integration bool
		checkedOut  bool
	}{
		{branch: "feature/merged", merged: true},
		{branch: "feature/squashed", merged: true}, // no ancestry, patch-equivalent
		{branch: "feature/stalled"},
		{branch: "feature/active", checkedOut: true},
		{branch: "main", merged: true, integration: true},
	} {
		got, err := FindBranch(f.dir, "", tc.branch)
		if err != nil {
			t.Fatalf("FindBranch(%q): %v", tc.branch, err)
		}
		if got.Merged != tc.merged || got.IsIntegration != tc.integration || got.CheckedOut != tc.checkedOut {
			t.Errorf("%s: merged=%v integration=%v checked-out=%v, want %v/%v/%v (%s)",
				tc.branch, got.Merged, got.IsIntegration, got.CheckedOut,
				tc.merged, tc.integration, tc.checkedOut, got.Evidence)
		}
		// This fixture has no remote at all, so every branch is local-only
		// — and a board that offered to push a deletion to an origin that
		// does not exist would be promising something it cannot do.
		if !got.Local || got.Remote {
			t.Errorf("%s: local=%v remote=%v, want a local-only branch", tc.branch, got.Local, got.Remote)
		}
	}

	if _, err := FindBranch(f.dir, "", "feature/never-existed"); err == nil {
		t.Error("FindBranch on a missing branch = nil error, want a refusal")
	}
	if _, err := FindBranch(f.dir, "not-a-spoke", "feature/merged"); err == nil ||
		!strings.Contains(err.Error(), "workspace") {
		t.Errorf("FindBranch in an undeclared repo = %v, want a message about the workspace", err)
	}
}
