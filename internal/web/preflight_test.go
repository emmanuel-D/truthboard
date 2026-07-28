package web

import (
	"bytes"
	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env builds a lookup over a fixed map, so the config checks are testable
// without touching the process environment.
func env(pairs map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := pairs[k]
		return v, ok
	}
}

func TestGitConfigEnvNamesTheIncompleteIndex(t *testing.T) {
	// The field failure: COUNT bumped to 2, the second VALUE never set. Git
	// answers "missing config value GIT_CONFIG_VALUE_1" from whatever
	// command ran next — which was `apk add` in an image build.
	err := checkGitConfigEnv(env(map[string]string{
		"GIT_CONFIG_COUNT":   "2",
		"GIT_CONFIG_KEY_0":   "url.https://oauth2:tok@gitlab.com/.insteadOf",
		"GIT_CONFIG_VALUE_0": "https://gitlab.com/",
		"GIT_CONFIG_KEY_1":   "url.https://oauth2:tok@gitlab.com/w/h.git.insteadOf",
	}))
	if err == nil {
		t.Fatal("an incomplete pair must not pass preflight")
	}
	if !strings.Contains(err.Error(), "GIT_CONFIG_VALUE_1") {
		t.Errorf("the error must name the missing half, got: %v", err)
	}
	if strings.Contains(err.Error(), "tok") {
		t.Errorf("the error leaked a credential: %v", err)
	}
}

func TestGitConfigEnvCatchesUncountedPair(t *testing.T) {
	// The quieter bug: the pair is right there, but the count never moved,
	// so git ignores it and the credential silently never applies.
	err := checkGitConfigEnv(env(map[string]string{
		"GIT_CONFIG_COUNT":   "1",
		"GIT_CONFIG_KEY_0":   "url.https://oauth2:tok@gitlab.com/.insteadOf",
		"GIT_CONFIG_VALUE_0": "https://gitlab.com/",
		"GIT_CONFIG_KEY_1":   "url.https://oauth2:tok@gitlab.com/w/h.git.insteadOf",
		"GIT_CONFIG_VALUE_1": "https://gitlab.com/w/h.git",
	}))
	if err == nil {
		t.Fatal("a pair beyond the count is ignored by git and must be reported")
	}
	if !strings.Contains(err.Error(), "ignores it") {
		t.Errorf("the error must say git ignores the pair, got: %v", err)
	}
}

func TestGitConfigEnvPassesWhenCoherentOrUnset(t *testing.T) {
	for name, pairs := range map[string]map[string]string{
		"unset": {},
		"zero":  {"GIT_CONFIG_COUNT": "0"},
		"complete": {
			"GIT_CONFIG_COUNT":   "1",
			"GIT_CONFIG_KEY_0":   "url.https://oauth2:tok@gitlab.com/.insteadOf",
			"GIT_CONFIG_VALUE_0": "https://gitlab.com/",
		},
		"empty value is still a value": {
			"GIT_CONFIG_COUNT":   "1",
			"GIT_CONFIG_KEY_0":   "core.pager",
			"GIT_CONFIG_VALUE_0": "",
		},
	} {
		if err := checkGitConfigEnv(env(pairs)); err != nil {
			t.Errorf("%s: must pass, got %v", name, err)
		}
	}
}

func TestGitConfigEnvRejectsNonCount(t *testing.T) {
	if err := checkGitConfigEnv(env(map[string]string{"GIT_CONFIG_COUNT": "two"})); err == nil {
		t.Fatal("a non-numeric count breaks every git command and must be caught")
	}
}

func TestRedactStripsCredentials(t *testing.T) {
	for in, want := range map[string]string{
		"https://oauth2:glpat-secret@gitlab.com/w/h.git": "https://***@gitlab.com/w/h.git",
		"https://x-access-token:ghp_s@github.com/o/r":    "https://***@github.com/o/r",
		"https://gitlab.com/w/h.git":                     "https://gitlab.com/w/h.git",
		"git@github.com:o/r.git":                         "git@github.com:o/r.git",
	} {
		if got := gitrepo.Redact(in); got != want {
			t.Errorf("gitrepo.Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExplainRemoteDistinguishesMissingFromWrongCredential(t *testing.T) {
	// A forge answers "no credential" and "wrong credential" identically,
	// and the two have opposite fixes — so preflight must separate them.
	t.Setenv("GIT_CONFIG_COUNT", "")
	const denied = "remote: HTTP Basic: Access denied.\nfatal: Authentication failed for 'https://gitlab.com/w/h.git/'"

	none := explainRemote("https://gitlab.com/w/h.git", denied)
	if !strings.Contains(none.Error(), "none was supplied") {
		t.Errorf("with no credential configured, say so: %v", none)
	}

	wrong := explainRemote("https://oauth2:tok@gitlab.com/w/h.git", denied)
	if !strings.Contains(wrong.Error(), "expired, or scoped without read access") {
		t.Errorf("with a credential present, blame the credential: %v", wrong)
	}
	if strings.Contains(wrong.Error(), "tok") {
		t.Errorf("the error leaked a credential: %v", wrong)
	}
}

func TestExplainRemotePassesThroughTheForgesOwnReason(t *testing.T) {
	// GitLab names the exact missing permission. Nothing this code could
	// invent beats that, so it must survive to the operator.
	err := explainRemote("https://oauth2:tok@gitlab.com/w/h.git",
		"remote: Access denied: This operation requires a fine-grained personal access token "+
			"with the following project permissions: [Code: Download].\n"+
			"fatal: unable to access 'https://gitlab.com/w/h.git/': The requested URL returned error: 403")
	if !strings.Contains(err.Error(), "[Code: Download]") {
		t.Errorf("the forge's own explanation must reach the operator, got: %v", err)
	}
}

func TestPreflightRejectsUnreachableRemote(t *testing.T) {
	dir := t.TempDir()
	if err := Preflight(filepath.Join(dir, "nope.git")); err == nil {
		t.Fatal("a remote that does not exist must fail preflight")
	}
}

func TestPreflightPassesOnAReadableRemote(t *testing.T) {
	_, reader := remoteFixture(t)
	origin := git(t, reader, "remote", "get-url", "origin")
	if err := Preflight(origin); err != nil {
		t.Fatalf("a readable remote must pass: %v", err)
	}
}

func TestPreflightRepoIsSilentWhenHealthy(t *testing.T) {
	// A working deploy gains no new boot output — the whole point is that
	// preflight only speaks when something is wrong.
	_, reader := remoteFixture(t)
	var out bytes.Buffer
	PreflightRepo(&out, reader, Options{Host: "0.0.0.0", EditToken: "s3cret"})
	if out.Len() > 0 {
		t.Errorf("a healthy repo must print nothing, got: %s", out.String())
	}
}

func TestPreflightRepoWarnsWhenEditingIsArmedWithoutPush(t *testing.T) {
	// The trap this closes: the board starts, serves, accepts the edit, and
	// only fails on the push — so the first person to save a story is the
	// one who discovers the deploy is broken.
	_, reader := remoteFixture(t)
	git(t, reader, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))

	var out bytes.Buffer
	PreflightRepo(&out, reader, Options{Host: "0.0.0.0", EditToken: "s3cret"})
	if !strings.Contains(out.String(), "cannot push") {
		t.Errorf("an unpushable clone with editing armed must warn, got: %q", out.String())
	}

	// Without an edit token there is nothing to warn about: the board never
	// writes, so push access is irrelevant.
	var quiet bytes.Buffer
	PreflightRepo(&quiet, reader, Options{Host: "0.0.0.0"})
	if strings.Contains(quiet.String(), "cannot push") {
		t.Errorf("a read-only board must not warn about push, got: %q", quiet.String())
	}
}

func TestPreflightRepoNamesUnreachableSpokes(t *testing.T) {
	_, reader := remoteFixture(t)
	_, other := remoteFixture(t)
	good := git(t, other, "remote", "get-url", "origin")

	manifest := "repos:\n" +
		"  good:\n    remote: " + filepath.ToSlash(good) + "\n" +
		"  broken:\n    remote: " + filepath.ToSlash(filepath.Join(t.TempDir(), "gone.git")) + "\n"
	if err := os.MkdirAll(filepath.Join(reader, ".truthboard"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reader, ".truthboard", "workspace.yml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	PreflightRepo(&out, reader, Options{})
	got := out.String()
	if !strings.Contains(got, "broken") {
		t.Errorf("the unreachable spoke must be named, got: %q", got)
	}
	if strings.Contains(got, "good:") {
		t.Errorf("a reachable spoke must not be reported, got: %q", got)
	}
}

// TestSyncErrorsNeverCarryACredential covers the path that made this
// worth doing: a spoke whose clone fails puts the error in the sync
// header, which the page prints as "⚠ remote sync failing: …". The clone
// takes its URL as an argument, so before redaction a manifest carrying a
// token published it to every viewer of a shared board.
func TestSyncErrorsNeverCarryACredential(t *testing.T) {
	const secret = "glpat-NEVER-PRINT-THIS"
	_, reader := remoteFixture(t)

	s := &syncer{
		repo:      filepath.Join(t.TempDir(), "spoke-clone"),
		remoteURL: "https://oauth2:" + secret + "@127.0.0.1:9/acme/spoke.git",
		name:      "spoke",
		proofOnly: true,
	}
	s.step()

	s.mu.Lock()
	got := s.err
	s.mu.Unlock()
	if got == "" {
		t.Fatal("the clone was expected to fail and record an error")
	}
	if strings.Contains(got, secret) {
		t.Errorf("the sync error carries a credential, and the board serves it in a header:\n%s", got)
	}
	if !strings.Contains(got, "***@") {
		t.Errorf("expected a redaction marker in:\n%s", got)
	}
	_ = reader
}
