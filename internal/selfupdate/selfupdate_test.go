package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fixture serves a fake release over HTTP and points the package at it,
// with gh disabled so the plain-HTTPS path is what's under test.
func fixture(t *testing.T, tag string, binary []byte, breakChecksum bool) (restore func(), exe string) {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	name := "truthboard"
	if runtime.GOOS == "windows" {
		name = "truthboard.exe"
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	tw.Write(binary)
	tw.Close()
	gz.Close()
	archive := buf.Bytes()

	assetName := fmt.Sprintf("truthboard_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])
	if breakChecksum {
		digest = strings.Repeat("0", 64)
	}
	sums := fmt.Sprintf("%s  %s\n", digest, assetName)

	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/repos/emmanuel-D/truthboard/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"tag_name":%q,"assets":[
			{"name":%q,"browser_download_url":%q},
			{"name":"checksums.txt","browser_download_url":%q}]}`,
			tag, assetName, srv.URL+"/dl/"+assetName, srv.URL+"/dl/checksums.txt")
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Base(r.URL.Path) {
		case assetName:
			w.Write(archive)
		case "checksums.txt":
			w.Write([]byte(sums))
		default:
			http.NotFound(w, r)
		}
	})
	srv = httptest.NewServer(mux)

	dir := t.TempDir()
	exe = filepath.Join(dir, "truthboard")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	oldAPI, oldExec, oldGH := apiBase, execPath, useGH
	apiBase = srv.URL
	execPath = func() (string, error) { return exe, nil }
	useGH = func() bool { return false }
	return func() {
		apiBase, execPath, useGH = oldAPI, oldExec, oldGH
		srv.Close()
	}, exe
}

func TestUpdateReplacesBinaryAtomically(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()

	var out bytes.Buffer
	if err := Run(&out, "v0.2.0", false); err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	raw, err := os.ReadFile(exe)
	if err != nil || string(raw) != "new binary" {
		t.Errorf("executable = %q (%v), want the new binary in place", raw, err)
	}
	info, _ := os.Stat(exe)
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		t.Error("replaced binary is not executable")
	}
	for _, want := range []string{"updated v0.2.0 → v9.9.9", "detached boards keep running the old binary"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestChecksumMismatchLeavesOldBinaryIntact(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("evil binary"), true)
	defer restore()

	var out bytes.Buffer
	err := Run(&out, "v0.2.0", false)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	raw, _ := os.ReadFile(exe)
	if string(raw) != "old binary" {
		t.Errorf("executable = %q, old binary must be untouched after a failed update", raw)
	}
}

func TestCheckOnlyTouchesNothing(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()

	var out bytes.Buffer
	if err := Run(&out, "v0.2.0", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "run `truthboard update` to install v9.9.9") {
		t.Errorf("check output = %q", out.String())
	}
	raw, _ := os.ReadFile(exe)
	if string(raw) != "old binary" {
		t.Errorf("--check must not modify the binary, got %q", raw)
	}
}

func TestDevBuildIsNeverReplaced(t *testing.T) {
	restore, exe := fixture(t, "v9.9.9", []byte("new binary"), false)
	defer restore()

	var out bytes.Buffer
	if err := Run(&out, "dev", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "source build") || !strings.Contains(out.String(), "go install") {
		t.Errorf("dev output = %q, want the source-build message", out.String())
	}
	raw, _ := os.ReadFile(exe)
	if string(raw) != "old binary" {
		t.Errorf("dev build must never be replaced, got %q", raw)
	}
}

func TestUpToDateSaysSo(t *testing.T) {
	restore, _ := fixture(t, "v0.2.0", []byte("x"), false)
	defer restore()

	var out bytes.Buffer
	if err := Run(&out, "v0.2.0", false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Errorf("output = %q", out.String())
	}
}
