// Package lifecycle manages the detached board process for a repo: start
// in the background, report status, stop. Runtime state lives inside the
// repo's git dir — per-repo, never committable, no system services.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// State describes a detached board.
type State struct {
	PID     int       `json:"pid"`
	Port    int       `json:"port"`
	URL     string    `json:"url"`
	Started time.Time `json:"started"`
}

func runDir(repo string) (string, error) {
	gitDir, err := gitrepo.Run(repo, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", fmt.Errorf("not a git repository: %s", repo)
	}
	return filepath.Join(gitDir, "truthboard"), nil
}

func statePath(repo string) (string, error) {
	dir, err := runDir(repo)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ui.json"), nil
}

// Load returns the recorded state, or nil when none exists.
func Load(repo string) (*State, error) {
	path, err := statePath(repo)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", path, err)
	}
	return &s, nil
}

func save(repo string, s *State) error {
	path, err := statePath(repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// Remove clears the recorded state.
func Remove(repo string) error {
	path, err := statePath(repo)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func logPath(repo string) string {
	dir, err := runDir(repo)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ui.log")
}

// Detach starts the board in its own session and records it. Refuses when
// a live board is already recorded; silently replaces stale state.
func Detach(repo string, port int, forge bool, version string) (*State, error) {
	if err := supported(); err != nil {
		return nil, err
	}
	if s, err := Load(repo); err != nil {
		return nil, err
	} else if s != nil {
		if Alive(s.PID) {
			return nil, fmt.Errorf("a board is already running at %s (pid %d) — `truthboard stop` first", s.URL, s.PID)
		}
		Remove(repo) // stale state from a crash or reboot
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dir, err := runDir(repo)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(logPath(repo), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	args := []string{"ui", "--no-open", "--port", fmt.Sprint(port)}
	if forge {
		args = append(args, "--forge")
	}
	args = append(args, repo)
	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = detachAttr()
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	s := &State{
		PID:     cmd.Process.Pid,
		Port:    port,
		URL:     fmt.Sprintf("http://127.0.0.1:%d", port),
		Started: time.Now(),
	}
	// The parent must not wait on the child, but the child must be
	// reparented rather than reaped by us.
	cmd.Process.Release()

	if err := waitReady(s.URL, 4*time.Second); err != nil {
		tail := logTail(repo, 5)
		return nil, fmt.Errorf("board did not come up: %v%s", err, tail)
	}
	// A 200 alone can come from whatever already held the port; the child
	// must still be alive for the answer to be ours.
	if !Alive(s.PID) {
		tail := logTail(repo, 5)
		return nil, fmt.Errorf("port %d answers but our board exited — something else is listening there (try --port)%s", port, tail)
	}
	if err := save(repo, s); err != nil {
		return nil, err
	}
	return s, nil
}

func waitReady(url string, budget time.Duration) error {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("no response within %s", budget)
}

func logTail(repo string, n int) string {
	raw, err := os.ReadFile(logPath(repo))
	if err != nil || len(raw) == 0 {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return "\n  " + strings.Join(lines, "\n  ")
}

// Status describes the recorded board in one line, cleaning stale state.
func Status(repo string) (string, error) {
	s, err := Load(repo)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "no detached board for this repo — start one with `truthboard ui --detach`", nil
	}
	if !Alive(s.PID) {
		Remove(repo)
		return fmt.Sprintf("stale state cleaned up (pid %d is gone) — start again with `truthboard ui --detach`", s.PID), nil
	}
	return fmt.Sprintf("running · %s · pid %d · up %s",
		s.URL, s.PID, time.Since(s.Started).Round(time.Second)), nil
}

// Stop terminates the detached board and clears state.
func Stop(repo string) (string, error) {
	if err := supported(); err != nil {
		return "", err
	}
	s, err := Load(repo)
	if err != nil {
		return "", err
	}
	if s == nil {
		return "nothing to stop — no detached board recorded for this repo", nil
	}
	if !Alive(s.PID) {
		Remove(repo)
		return fmt.Sprintf("board (pid %d) was already gone; state cleaned up", s.PID), nil
	}
	if err := terminate(s.PID, 2*time.Second); err != nil {
		return "", err
	}
	Remove(repo)
	return fmt.Sprintf("stopped the board at %s (pid %d)", s.URL, s.PID), nil
}
