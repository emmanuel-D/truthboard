//go:build !windows

package lifecycle

import (
	"fmt"
	"syscall"
	"time"
)

func supported() error { return nil }

// detachAttr puts the child in its own session so closing the terminal
// never takes the board down with it.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// Alive reports whether the process exists (signal 0 probe).
func Alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// terminate asks politely, then insists.
func terminate(pid int, grace time.Duration) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}
