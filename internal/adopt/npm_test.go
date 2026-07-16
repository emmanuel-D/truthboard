package adopt

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func npmRepo(t *testing.T, pkg string) string {
	t.Helper()
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not on PATH")
	}
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readScripts(t *testing.T, dir string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Name    string            `json:"name"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	return pkg.Scripts
}

func TestNpmScriptsAddedAndIdempotent(t *testing.T) {
	dir := npmRepo(t, `{"name":"demo","version":"1.0.0","scripts":{"dev":"vite"}}`)

	log, err := NpmScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(log, "\n"), "added scripts board, board:status, board:stop, board:audit") {
		t.Errorf("log = %v", log)
	}
	scripts := readScripts(t, dir)
	if scripts["board"] != "truthboard ui --detach" || scripts["dev"] != "vite" {
		t.Errorf("scripts = %v (must add ours, keep theirs)", scripts)
	}

	log, err = NpmScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(log, "\n")
	if !strings.Contains(joined, "already there") || strings.Contains(joined, "added") {
		t.Errorf("second run must be a no-op, log = %v", log)
	}
}

func TestNpmScriptsNeverOverwrite(t *testing.T) {
	dir := npmRepo(t, `{"name":"demo","version":"1.0.0","scripts":{"board":"echo my own board"}}`)

	log, err := NpmScripts(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(log, "\n"), `kept your existing script board`) {
		t.Errorf("log = %v", log)
	}
	if got := readScripts(t, dir)["board"]; got != "echo my own board" {
		t.Errorf("board script = %q — an existing script was overwritten", got)
	}
}

func TestNpmScriptsSkipsGracefully(t *testing.T) {
	log, err := NpmScripts(gitRepo(t)) // no package.json
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(log, "\n"), "skipped") {
		t.Errorf("log = %v", log)
	}
}
