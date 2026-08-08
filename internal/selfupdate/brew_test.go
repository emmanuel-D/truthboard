package selfupdate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// brewKeg relocates the fixture's binary into a Homebrew-shaped path and
// points the package at it, returning that path.
func brewKeg(t *testing.T, exe, formula string) string {
	t.Helper()
	cellar := filepath.Join(filepath.Dir(exe), "Cellar", formula, "0.12.2", "bin")
	if err := os.MkdirAll(cellar, 0o755); err != nil {
		t.Fatal(err)
	}
	keg := filepath.Join(cellar, "truthboard")
	if err := os.WriteFile(keg, []byte("keg binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	execPath = func() (string, error) { return keg, nil } // fixture's restore puts this back
	return keg
}

// A keg belongs to brew. Swapping it in place would leave brew reporting the
// old version and revert the update on its next upgrade.
func TestBrewKegIsRedirectedNotReplaced(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()
	keg := brewKeg(t, exe, "truthboard")

	var out bytes.Buffer
	if err := Run(&out, "v0.12.2", false); err != nil {
		t.Fatalf("a brew install is guidance, not a failure: %v", err)
	}
	raw, err := os.ReadFile(keg)
	if err != nil || string(raw) != "keg binary" {
		t.Errorf("the keg was modified: %q (%v)", raw, err)
	}
	for _, want := range []string{"managed by Homebrew", "brew update && brew upgrade truthboard"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "updated v0.12.2") {
		t.Errorf("output claims an update that did not happen:\n%s", out.String())
	}
}

// --check must give the advice that works, not send the user to a command
// that will only refuse.
func TestBrewKegCheckAdvisesBrew(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()
	brewKeg(t, exe, "truthboard")

	var out bytes.Buffer
	if err := Run(&out, "v0.12.2", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "brew upgrade truthboard") {
		t.Errorf("--check should name the brew command:\n%s", out.String())
	}
	if strings.Contains(out.String(), "run `truthboard update`") {
		t.Errorf("--check sent the user to a command that refuses:\n%s", out.String())
	}
}

// Nothing to do means nothing to say: no warning where no update exists.
func TestBrewKegUpToDateStaysQuiet(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()
	brewKeg(t, exe, "truthboard")

	var out bytes.Buffer
	if err := Run(&out, "v9.9.9", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("want the plain up-to-date line:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Homebrew") {
		t.Errorf("nothing to update, so nothing to warn about:\n%s", out.String())
	}
}

// The formula name is read from the path, so a tap's formula name travels.
func TestBrewKegUsesTheFormulaName(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()
	brewKeg(t, exe, "truthboard-edge")

	var out bytes.Buffer
	if err := Run(&out, "v0.12.2", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "brew upgrade truthboard-edge") {
		t.Errorf("want the real formula name:\n%s", out.String())
	}
}

// Every non-brew install must keep updating in place.
func TestOrdinaryInstallStillUpdates(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()

	var out bytes.Buffer
	if err := Run(&out, "v0.12.2", false); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(exe); string(raw) != "new binary" {
		t.Errorf("a script install must still be replaced, got %q", raw)
	}
	if strings.Contains(out.String(), "Homebrew") {
		t.Errorf("a plain install is not a keg:\n%s", out.String())
	}
}

// Detection is structural: the keg layout, not the word "Cellar".
func TestBrewFormulaDetection(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		path string
		want string
	}{
		{filepath.Join(sep, "opt", "homebrew", "Cellar", "truthboard", "0.12.2", "bin", "truthboard"), "truthboard"},
		{filepath.Join(sep, "home", "linuxbrew", ".linuxbrew", "Cellar", "truthboard", "0.1.0", "bin", "truthboard"), "truthboard"},
		{filepath.Join(sep, "usr", "local", "bin", "truthboard"), ""},
		{filepath.Join(sep, "Users", "me", "go", "bin", "truthboard"), ""},
		// A directory that merely has the name, at the wrong depth.
		{filepath.Join(sep, "Cellar", "truthboard"), ""},
		{filepath.Join(sep, "wine", "Cellar", "bin", "truthboard"), ""},
		// Right shape, wrong leaf directory.
		{filepath.Join(sep, "opt", "Cellar", "truthboard", "0.1.0", "libexec", "truthboard"), ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := brewFormula(c.path); got != c.want {
			t.Errorf("brewFormula(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
