package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// callWithBanner drives one tools/call through handle with a stale server,
// and returns the content blocks of the answer.
func callWithBanner(t *testing.T, repo, tool, args, banner string) []any {
	t.Helper()
	req := request{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"` + tool + `","arguments":` + args + `}`),
	}
	resp := handle(req, repo, "v0.16.0", func(string) string { return banner })
	if resp == nil {
		t.Fatal("no response")
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("no result: %+v", resp)
	}
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("%s failed: %+v", tool, result)
	}
	return toAny(result["content"])
}

// toAny normalises handle's []map[string]any content to []any.
func toAny(content any) []any {
	blocks, ok := content.([]map[string]any)
	if !ok {
		return nil
	}
	out := make([]any, len(blocks))
	for i, b := range blocks {
		out[i] = any(b)
	}
	return out
}

// A stale server still answers everything, and the board it answers with is
// still parseable: the warning rides as a second content block, never glued
// to the front of JSON no client could then read.
func TestBannerRidesBesideTheBoardNotInsideIt(t *testing.T) {
	repo := fixtureRepo(t)
	const banner = "⚠ this MCP server is truthboard v0.16.0"

	content := callWithBanner(t, repo, "get_board", `{}`, banner)
	if len(content) != 2 {
		t.Fatalf("want the board and the warning as two blocks, got %d", len(content))
	}
	board := content[0].(map[string]any)["text"].(string)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(board), &parsed); err != nil {
		t.Fatalf("the board must stay parseable JSON: %v\n%s", err, board)
	}
	if parsed["specs"] == nil {
		t.Error("the board must still carry its specs")
	}
	if warn := content[1].(map[string]any)["text"].(string); warn != banner {
		t.Errorf("second block = %q, want the warning", warn)
	}
}

// Intent writes are not status answers, and a warning on every call is one
// nobody reads by the third.
func TestBannerStaysOffIntentWrites(t *testing.T) {
	repo := fixtureRepo(t)
	content := callWithBanner(t, repo, "create_spec", `{"title":"A story"}`, "⚠ stale")
	if len(content) != 1 {
		t.Errorf("create_spec must answer with one block, got %d", len(content))
	}
}

// A current server adds nothing at all — silence is what makes the warning
// mean something when it does appear.
func TestCurrentServerAddsNothing(t *testing.T) {
	repo := fixtureRepo(t)
	content := callWithBanner(t, repo, "get_board", `{}`, "")
	if len(content) != 1 {
		t.Errorf("a current server must add nothing, got %d blocks", len(content))
	}
}

// The comparison is the whole product here: a warning that fires when it
// should not is one an agent learns to ignore, and then the one that matters
// goes unread too.
func TestStaleBannerFiresOnlyWhenSuperseded(t *testing.T) {
	for _, tc := range []struct {
		name      string
		serving   string
		installed string
		warn      bool
	}{
		{"older patch", "v0.17.0", "v0.17.1", true},
		{"older minor", "v0.16.0", "v0.17.0", true},
		{"older major", "v0.9.9", "v1.0.0", true},
		{"equal", "v0.17.0", "v0.17.0", false},
		{"newer than installed", "v0.18.0", "v0.17.0", false},
		// A build from a checkout reports "dev" and is deliberate — its owner
		// does not need telling that a release exists.
		{"dev build serving", "dev", "v0.17.0", false},
		{"dev build installed", "v0.16.0", "dev", false},
		// Nothing on PATH, or a truthboard that would not answer.
		{"nothing installed", "v0.16.0", "", false},
		{"nothing serving", "", "v0.17.0", false},
		// Unparseable on either side: unanswerable, not old.
		{"pre-release serving", "v0.17.0-rc1", "v0.18.0", false},
		{"pre-release installed", "v0.16.0", "v0.17.0-rc1", false},
		{"pseudo-version", "v0.8.4-0.2026-439a3a04fae7+dirty", "v0.17.0", false},
		{"not a version at all", "v0.16.0", "truthboard", false},
		{"too few parts", "v0.16", "v0.17", false},
		{"negative", "v0.16.0", "v0.-1.0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := staleBanner(tc.serving, tc.installed)
			if (got != "") != tc.warn {
				t.Fatalf("staleBanner(%q, %q) = %q, want warn=%v", tc.serving, tc.installed, got, tc.warn)
			}
			if !tc.warn {
				return
			}
			// Both versions, or the reader cannot tell what to restart.
			for _, want := range []string{tc.serving, tc.installed, "Restart your MCP client", "truthboard audit"} {
				if !strings.Contains(got, want) {
					t.Errorf("banner must name %q:\n%s", want, got)
				}
			}
		})
	}
}

// The probe costs a process spawn. It answers a question that cannot change
// while the server runs — this build is this build — so it is paid once.
func TestStalenessProbesOnce(t *testing.T) {
	calls := 0
	s := staleness{probe: func() string { calls++; return "v9.9.9" }}

	first := s.warning("v0.1.0")
	if first == "" {
		t.Fatal("v0.1.0 against v9.9.9 must warn")
	}
	for range 4 {
		if again := s.warning("v0.1.0"); again != first {
			t.Errorf("banner changed on a later call:\n%s\n%s", first, again)
		}
	}
	if calls != 1 {
		t.Errorf("probed %d times, want once per server lifetime", calls)
	}
}

// Only the tools whose answer is a derived status. Repeating the banner on
// every intent write is how it stops being read.
func TestOnlyStatusBearingToolsCarryTheBanner(t *testing.T) {
	for _, name := range []string{"get_board", "next_spec", "list_specs", "get_brief"} {
		if !statusBearing[name] {
			t.Errorf("%s answers with a derived status and must carry the warning", name)
		}
	}
	for _, name := range []string{"create_spec", "update_spec", "check_acceptance"} {
		if statusBearing[name] {
			t.Errorf("%s writes intent — banner-worthy only as noise", name)
		}
	}
}
