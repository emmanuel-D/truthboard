package forge

import (
	"encoding/json"
	"testing"
)

// Trimmed real-shape GitLab API payloads.
const sampleMRs = `[
  {"iid": 42, "title": "feat: scoreboard", "state": "opened", "draft": false,
   "source_branch": "feat/scoreboard", "updated_at": "2026-07-10T10:00:00Z"},
  {"iid": 41, "title": "wip: physics", "state": "opened", "draft": true,
   "source_branch": "wip/physics", "updated_at": "2026-07-09T10:00:00Z"},
  {"iid": 40, "title": "fix: crash", "state": "merged", "draft": false,
   "source_branch": "fix/crash", "updated_at": "2026-07-01T10:00:00Z"},
  {"iid": 39, "title": "spike: webgpu", "state": "closed", "draft": false,
   "source_branch": "spike/webgpu", "updated_at": "2026-06-01T10:00:00Z"}
]`

const sampleIssues = `[
  {"iid": 7, "title": "Bots too passive", "state": "opened",
   "updated_at": "2026-07-01T10:00:00Z", "assignees": [{"username": "emmanuel"}]},
  {"iid": 6, "title": "Wishlist item", "state": "opened",
   "updated_at": "2026-01-01T10:00:00Z", "assignees": []},
  {"iid": 5, "title": "Old bug", "state": "closed",
   "updated_at": "2026-05-01T10:00:00Z", "assignees": []}
]`

func TestConvertGitLabMRs(t *testing.T) {
	var mrs []glabMR
	if err := json.Unmarshal([]byte(sampleMRs), &mrs); err != nil {
		t.Fatal(err)
	}
	prs := convertGitLabMRs(mrs)

	want := map[int]struct {
		state string
		draft bool
		head  string
	}{
		42: {"OPEN", false, "feat/scoreboard"},
		41: {"OPEN", true, "wip/physics"},
		40: {"MERGED", false, "fix/crash"},
		39: {"CLOSED", false, "spike/webgpu"},
	}
	if len(prs) != len(want) {
		t.Fatalf("got %d PRs, want %d", len(prs), len(want))
	}
	for _, pr := range prs {
		w := want[pr.Number]
		if pr.State != w.state || pr.IsDraft != w.draft || pr.HeadRefName != w.head {
			t.Errorf("MR !%d = %+v, want %+v", pr.Number, pr, w)
		}
	}
}

func TestConvertGitLabIssues(t *testing.T) {
	var issues []glabIssue
	if err := json.Unmarshal([]byte(sampleIssues), &issues); err != nil {
		t.Fatal(err)
	}
	out := convertGitLabIssues(issues)

	byNumber := map[int]Issue{}
	for _, is := range out {
		byNumber[is.Number] = is
	}
	if is := byNumber[7]; is.State != "OPEN" || !is.Assigned() || is.Assignees[0].Login != "emmanuel" {
		t.Errorf("#7 = %+v, want open and assigned to emmanuel", is)
	}
	if is := byNumber[6]; is.State != "OPEN" || is.Assigned() {
		t.Errorf("#6 = %+v, want open and unassigned (backlog)", is)
	}
	if is := byNumber[5]; is.State != "CLOSED" {
		t.Errorf("#5 = %+v, want closed", is)
	}
}

func TestGitLabRepoNameParsing(t *testing.T) {
	for url, want := range map[string]string{
		"git@gitlab.com:retropixel3d/retropixel3d-mono.git": "retropixel3d/retropixel3d-mono",
		"https://gitlab.com/group/sub/project.git":          "group/sub/project",
		"https://gitlab.example.com/team/app":               "team/app",
		"ssh://git@gitlab.com/ns/proj.git":                  "ns/proj",
	} {
		if m := remotePathPattern.FindStringSubmatch(url); m == nil || m[1] != want {
			got := "<no match>"
			if m != nil {
				got = m[1]
			}
			t.Errorf("parse(%q) = %q, want %q", url, got, want)
		}
	}
}
