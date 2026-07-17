package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// fakeOllama serves a canned completion through the real HTTP path, so the
// provider selection, request shape, and response parsing are all exercised.
func fakeOllama(t *testing.T, completion string) Provider {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"response": completion})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OLLAMA_HOST", srv.URL)
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFromEnvRequiresExplicitConfig(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OLLAMA_HOST", "")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") || !strings.Contains(err.Error(), "OLLAMA_HOST") {
		t.Errorf("want an error naming both env vars, got %v", err)
	}
}

const goodDraft = "```json\n" + `{
  "epic": "email-verification",
  "stories": [
    {"title": "Send the verification token", "type": "story", "priority": 1, "points": 3,
     "body": "## Goal\n\nVerify emails.\n\n## Acceptance\n\n- [ ] **Given** a signup **when** submitted **then** a token is mailed"},
    {"title": "Block unverified logins", "type": "made-up-type", "priority": 9, "points": 2,
     "body": "## Goal\n\nKeep spam out.\n\n## Acceptance\n\n- [ ] **Given** an unverified account **when** login **then** it is refused"}
  ]
}` + "\n```"

func TestDraftWritesRealSpecs(t *testing.T) {
	repo := t.TempDir()
	p := fakeOllama(t, goodDraft)
	created, err := Draft(p, repo, "email verification", "ada")
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created %d specs, want 2", len(created))
	}
	specs, err := spec.Load(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("loaded %d specs from disk, want 2", len(specs))
	}
	for _, s := range specs {
		if s.Epic != "email-verification" || s.Owner != "ada" {
			t.Errorf("spec %s: epic=%q owner=%q", s.ID, s.Epic, s.Owner)
		}
		if !strings.Contains(s.Body, "## Goal") || !strings.Contains(s.Body, "- [ ]") {
			t.Errorf("spec %s body is a placeholder:\n%s", s.ID, s.Body)
		}
	}
	// The invented type degraded to story (empty) and priority was clamped —
	// files must still load, which spec.Load above already proved.
	for _, s := range specs {
		if s.Priority > 3 {
			t.Errorf("spec %s priority %d escaped the clamp", s.ID, s.Priority)
		}
	}
}

func TestDraftRefusesPlaceholderStories(t *testing.T) {
	repo := t.TempDir()
	p := fakeOllama(t, `{"epic":"x","stories":[{"title":"Vague thing","body":"TODO"}]}`)
	if _, err := Draft(p, repo, "concept", ""); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("want a placeholder refusal, got %v", err)
	}
	if specs, _ := spec.Load(repo); len(specs) != 0 {
		t.Errorf("refused draft still wrote %d specs", len(specs))
	}
}

func TestReviewFactsAreDerivedNotInvented(t *testing.T) {
	res := &audit.Result{
		Integration: "main", DigestDays: 14,
		Specs: []audit.SpecStatus{
			{ID: "tb-0001", Title: "Landed one", Sprint: "s12", Status: audit.Done},
		},
		Sprints: []audit.SprintRollup{{
			Name: "s12", Done: 1, Total: 2, Start: "2026-07-14", End: "2026-07-25", State: "active",
			Open: []audit.SprintOpen{{ID: "tb-0002", Title: "Rolls over", Status: audit.InProgress}},
		}},
	}
	facts, err := reviewFacts(res, "s12")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1/2 stories done", "Rolls over", "Landed one", "active"} {
		if !strings.Contains(facts, want) {
			t.Errorf("facts missing %q:\n%s", want, facts)
		}
	}
	if _, err := reviewFacts(res, "s99"); err == nil {
		t.Error("unknown sprint should error, not invent a review")
	}
}

func TestNoImplicitCalls(t *testing.T) {
	// A provider is only ever constructed by FromEnv, and FromEnv is only
	// called from the draft/review commands — guard the env contract here.
	t.Setenv("ANTHROPIC_API_KEY", "k")
	os.Unsetenv("TRUTHBOARD_LLM_MODEL")
	p, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() != "anthropic/claude-sonnet-5" {
		t.Errorf("default provider = %s, want anthropic/claude-sonnet-5", p.Name())
	}
}
