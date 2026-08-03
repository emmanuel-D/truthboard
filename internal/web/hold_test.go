package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The write gate admits PUT on /api/specs/<id>, not PATCH — anything else
// is refused as an attempt to type a status.
func put(t *testing.T, url, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("PUT", url, strings.NewReader(body))
	req.Header.Set("X-Truthboard-Token", "s3cret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The board's own editor must be able to write and clear a hold, and the
// board it serves back must carry the note for every viewer.
func TestBoardEditorWritesAndClearsAHold(t *testing.T) {
	_, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	resp := post(t, srv.URL+"/api/specs", `{"title":"EU tax rates","hold":"waiting on legal sign-off"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("create with a hold = %d, want 200", resp.StatusCode)
	}
	var created struct {
		ID   string `json:"id"`
		Hold string `json:"hold"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Hold != "waiting on legal sign-off" {
		t.Fatalf("created payload hold = %q, want the note echoed back", created.Hold)
	}

	if !boardHas(t, srv.URL, `"hold":"waiting on legal sign-off"`) {
		t.Error("the board does not carry the hold; viewers would see a pause with no reason")
	}

	// Clearing is sending the empty string — deleting the line, not an
	// "unhold" verb.
	resp = put(t, srv.URL+"/api/specs/"+created.ID, `{"hold":""}`)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("clearing the hold = %d, want 200", resp.StatusCode)
	}
	if boardHas(t, srv.URL, `"hold"`) {
		t.Error("cleared hold still on the board")
	}
}

// A payload naming a status must still fail loudly — a hold does not open
// a side door to typing one.
func TestHoldDoesNotOpenAStatusDoor(t *testing.T) {
	_, clone := originAndClone(t)
	srv := httptest.NewServer(Handler(clone, Options{Host: "0.0.0.0", EditToken: "s3cret", Version: "test"}))
	defer srv.Close()

	resp := post(t, srv.URL+"/api/specs", `{"title":"x","hold":"paused","status":"on-hold"}`)
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		t.Error("a payload carrying a status was accepted; unknown fields must be rejected")
	}
}

func boardHas(t *testing.T, base, want string) bool {
	t.Helper()
	resp, err := http.Get(base + "/api/board")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(body), want)
}
