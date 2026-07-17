// Package selfupdate replaces the running truthboard binary with the
// latest GitHub release. The release feed is resolved through `gh` when
// available (which also works while the repo is private) and the public
// API otherwise; downloads are verified against the release's
// checksums.txt before anything is touched, and the swap is atomic — a
// failed update leaves the old binary exactly where it was.
package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repoSlug = "emmanuel-D/truthboard"

// Overridable for tests: the API host and how the executable is located.
var (
	apiBase  = "https://api.github.com"
	execPath = os.Executable
	useGH    = func() bool { _, err := exec.LookPath("gh"); return err == nil }
)

type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Run checks for (and unless checkOnly, installs) the latest release.
func Run(w io.Writer, current string, checkOnly bool) error {
	rel, err := latest()
	if err != nil {
		return fmt.Errorf("cannot read the release feed: %w", err)
	}
	fmt.Fprintf(w, "current: %s · latest release: %s\n", current, rel.Tag)

	if current == rel.Tag {
		fmt.Fprintln(w, "already up to date")
		return nil
	}
	if current == "dev" {
		fmt.Fprintln(w, "this is a source build — it is never replaced by update.")
		fmt.Fprintln(w, "update from source:  git pull && go install ./cmd/truthboard")
		return nil
	}
	if checkOnly {
		fmt.Fprintf(w, "run `truthboard update` to install %s\n", rel.Tag)
		return nil
	}

	assetName := fmt.Sprintf("truthboard_%s_%s_%s.tar.gz", rel.Tag, runtime.GOOS, runtime.GOARCH)
	archive, err := download(rel, assetName)
	if err != nil {
		return err
	}
	sums, err := download(rel, "checksums.txt")
	if err != nil {
		return fmt.Errorf("release %s has no readable checksums.txt — refusing to install unverified bits: %w", rel.Tag, err)
	}
	if err := verify(archive, assetName, sums); err != nil {
		return err
	}
	bin, err := extractBinary(archive)
	if err != nil {
		return err
	}
	path, err := replaceExecutable(bin)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "updated %s → %s (%s)\n", current, rel.Tag, path)
	fmt.Fprintln(w, "note: detached boards keep running the old binary — in each repo:")
	fmt.Fprintln(w, "  truthboard stop && truthboard ui --detach")
	return nil
}

// latest resolves the newest release, preferring gh (authenticated, so it
// also works on a private repo) over the anonymous public API.
func latest() (*release, error) {
	if useGH() {
		out, err := exec.Command("gh", "api", "repos/"+repoSlug+"/releases/latest").Output()
		if err == nil {
			var rel release
			if json.Unmarshal(out, &rel) == nil && rel.Tag != "" {
				return &rel, nil
			}
		}
		// fall through: gh may be present but unauthenticated
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(apiBase + "/repos/" + repoSlug + "/releases/latest")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s (private repo without gh, or no releases yet?)", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	if rel.Tag == "" {
		return nil, fmt.Errorf("release feed had no tag")
	}
	return &rel, nil
}

// download fetches a named asset, via gh when available (asset URLs on a
// private repo are not anonymously reachable).
func download(rel *release, name string) ([]byte, error) {
	if useGH() {
		dir, err := os.MkdirTemp("", "truthboard-update-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		cmd := exec.Command("gh", "release", "download", rel.Tag,
			"--repo", repoSlug, "--pattern", name, "--dir", dir)
		if out, err := cmd.CombinedOutput(); err == nil {
			if raw, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				return raw, nil
			}
		} else {
			_ = out // fall through to plain HTTPS
		}
	}
	for _, a := range rel.Assets {
		if a.Name != name {
			continue
		}
		client := &http.Client{Timeout: 2 * time.Minute}
		resp, err := client.Get(a.URL)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("downloading %s: %s", name, resp.Status)
		}
		return io.ReadAll(resp.Body)
	}
	return nil, fmt.Errorf("release %s has no asset %q for this platform", rel.Tag, name)
}

// verify checks the archive against its line in checksums.txt.
func verify(archive []byte, name string, sums []byte) error {
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			want = fields[0]
		}
	}
	if want == "" {
		return fmt.Errorf("checksums.txt has no entry for %s — refusing to install unverified bits", name)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("checksum mismatch for %s — the download is corrupt or tampered with; nothing was changed", name)
	}
	return nil
}

// extractBinary pulls the single truthboard binary out of the tarball.
func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		base := filepath.Base(hdr.Name)
		if base == "truthboard" || base == "truthboard.exe" {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("archive contains no truthboard binary")
}

// replaceExecutable swaps the running binary atomically: the new one is
// written next to it and renamed into place (the old file is first moved
// aside, which also keeps Windows happy about replacing a running exe).
func replaceExecutable(bin []byte) (string, error) {
	path, err := execPath()
	if err != nil {
		return "", err
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".truthboard-new-*")
	if err != nil {
		return "", fmt.Errorf("cannot write next to %s (permissions?): %w", path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after the successful rename
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	old := path + ".old"
	if err := os.Rename(path, old); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Rename(old, path) // put the old binary back; never leave a hole
		return "", err
	}
	os.Remove(old) // best effort; Windows may keep it until the process exits
	return path, nil
}
