package gitrepo

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactStripsCredentialsAndLeavesTheRestAlone(t *testing.T) {
	for in, want := range map[string]string{
		// The two documented forms.
		"https://oauth2:glpat-secret@gitlab.com/w/h.git": "https://***@gitlab.com/w/h.git",
		"https://x-access-token:ghp_s@github.com/o/r":    "https://***@github.com/o/r",
		// Nothing to hide.
		"https://gitlab.com/w/h.git": "https://gitlab.com/w/h.git",
		// scp-style carries no secret, and an @ in a path is not userinfo.
		"git@github.com:o/r.git":            "git@github.com:o/r.git",
		"https://gitlab.com/w/h/@latest.md": "https://gitlab.com/w/h/@latest.md",
		// Prose around a URL, which is what an error actually looks like.
		"git clone --mirror https://oauth2:tok@gitlab.com/a/b.git /repo: failed": "git clone --mirror https://***@gitlab.com/a/b.git /repo: failed",
		// More than one in a line.
		"a https://u:p@x.com/1 and https://u:p@y.com/2": "a https://***@x.com/1 and https://***@y.com/2",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q)\n = %q\nwant %q", in, got, want)
		}
	}
}

// TestRunNeverEchoesACredential is the guarantee the rest of the tool
// leans on: every git failure in this codebase is formatted here, so if a
// token cannot escape this function it cannot reach a log or a board page.
func TestRunNeverEchoesACredential(t *testing.T) {
	repo := t.TempDir()
	if out, err := Run(repo, "init", "--quiet", "-b", "main"); err != nil {
		t.Fatalf("init: %v %s", err, out)
	}
	const secret = "glpat-NEVER-PRINT-THIS"

	// A clone takes the URL as an argument, so the failure formats it back.
	// Port 9 (discard) refuses fast and deterministically.
	_, err := Run(repo, "clone", "--mirror",
		"https://oauth2:"+secret+"@127.0.0.1:9/acme/hub.git", filepath.Join(repo, "dest"))
	if err == nil {
		t.Fatal("the clone was expected to fail")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("a credential reached the error text, which a shared board serves to anyone:\n%v", err)
	}
	if !strings.Contains(err.Error(), "***@127.0.0.1:9") {
		t.Errorf("the host must survive redaction — an error naming no remote helps nobody:\n%v", err)
	}
}

func TestRedactHandlesEmptyAndGarbage(t *testing.T) {
	for _, in := range []string{"", "not a url", "://", "@", "https://"} {
		if got := Redact(in); strings.Contains(got, "\x00") {
			t.Errorf("Redact(%q) = %q", in, got)
		}
	}
}
