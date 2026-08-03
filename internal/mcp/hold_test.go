package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// A hold is intent, so it must round-trip through the same write path as
// every other intent field — and clearing it must remove the line, since
// there is no unhold verb.
func TestAgentWritesAndClearsAHold(t *testing.T) {
	repo := fixtureRepo(t)
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"EU tax rates","hold":"waiting on legal sign-off","epic":"invoicing"}}}`,
	)
	text, isErr := toolText(t, responses[0])
	if isErr {
		t.Fatalf("create_spec with a hold failed: %s", text)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(text), &created); err != nil {
		t.Fatal(err)
	}

	content := readSpecFile(t, repo, created.ID)
	if !strings.Contains(content, "hold: waiting on legal sign-off") {
		t.Fatalf("hold not written to the spec file:\n%s", content)
	}

	// Rewriting the reason replaces it. Driven separately from the clear
	// below: drive() runs every request before returning, so inspecting the
	// file between two batched writes would only ever see the last one.
	responses = drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"`+created.ID+`","hold":"legal replied, waiting on finance"}}}`,
	)
	if text, isErr := toolText(t, responses[0]); isErr {
		t.Fatalf("update_spec hold failed: %s", text)
	}
	if content := readSpecFile(t, repo, created.ID); !strings.Contains(content, "legal replied, waiting on finance") {
		t.Errorf("rewritten hold missing:\n%s", content)
	}

	// The empty string clears it — there is no unhold verb.
	responses = drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"update_spec","arguments":{"id":"`+created.ID+`","hold":""}}}`,
	)
	if text, isErr := toolText(t, responses[0]); isErr {
		t.Fatalf("clearing the hold failed: %s", text)
	}
	if content := readSpecFile(t, repo, created.ID); strings.Contains(content, "hold:") {
		t.Errorf("cleared hold still present — clearing must delete the line:\n%s", content)
	}
}

// A hold is not a status: it must never become a way to type one.
func TestHoldDoesNotSmuggleInAStatus(t *testing.T) {
	repo := fixtureRepo(t)
	responses := drive(t, repo,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_spec","arguments":{"title":"Held story","hold":"on hold"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_board","arguments":{}}}`,
	)
	if text, isErr := toolText(t, responses[0]); isErr {
		t.Fatalf("create_spec failed: %s", text)
	}
	text, isErr := toolText(t, responses[1])
	if isErr {
		t.Fatalf("get_board failed: %s", text)
	}
	if strings.Contains(text, `"status": "on-hold"`) || strings.Contains(text, `"status": "hold"`) {
		t.Errorf("a hold invented a status:\n%.400s", text)
	}
	if !strings.Contains(text, `"status": "planned"`) {
		t.Errorf("held story lost its derived status:\n%.400s", text)
	}
}

func readSpecFile(t *testing.T, repo, id string) string {
	t.Helper()
	raw, err := os.ReadFile(specFileByID(t, repo, id))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
