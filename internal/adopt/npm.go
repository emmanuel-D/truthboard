package adopt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// boardScripts is what `npm run <name>` should do in a truthboard repo.
// Order matters only for the report.
var boardScripts = [][2]string{
	{"board", "truthboard ui --detach"},
	{"board:status", "truthboard status"},
	{"board:stop", "truthboard stop"},
	{"board:audit", "truthboard audit"},
}

// NpmScripts wires the board lifecycle into package.json via `npm pkg`,
// so npm itself rewrites the file and we never hand-mangle it. Missing
// package.json or npm is a note, never an error; existing scripts are
// never overwritten.
func NpmScripts(repo string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(repo, "package.json")); err != nil {
		return []string{"package.json: none, npm scripts skipped"}, nil
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return []string{"package.json found but npm is not on PATH — scripts skipped"}, nil
	}

	var added, kept, skipped []string
	for _, s := range boardScripts {
		name, cmd := s[0], s[1]
		current, exists := npmPkgGet(repo, "scripts."+name)
		switch {
		case exists && current == cmd:
			kept = append(kept, name)
		case exists:
			skipped = append(skipped, fmt.Sprintf("%s (already %q)", name, current))
		default:
			if err := npmPkgSet(repo, "scripts."+name, cmd); err != nil {
				return nil, fmt.Errorf("npm pkg set scripts.%s: %w", name, err)
			}
			added = append(added, name)
		}
	}

	var log []string
	if len(added) > 0 {
		log = append(log, "package.json: added scripts "+strings.Join(added, ", "))
	}
	if len(kept) > 0 {
		log = append(log, "package.json: scripts already there: "+strings.Join(kept, ", "))
	}
	for _, s := range skipped {
		log = append(log, "package.json: kept your existing script "+s)
	}
	return log, nil
}

// npmPkgGet returns the current value of a key, and whether it is set.
func npmPkgGet(repo, key string) (string, bool) {
	cmd := exec.Command("npm", "pkg", "get", key)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(out))
	if v == "{}" || v == "undefined" {
		return "", false
	}
	return strings.Trim(v, `"`), true
}

func npmPkgSet(repo, key, value string) error {
	cmd := exec.Command("npm", "pkg", "set", key+"="+value)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
