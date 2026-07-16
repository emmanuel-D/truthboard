//go:build windows

package lifecycle

import (
	"errors"
	"syscall"
	"time"
)

var errUnsupported = errors.New("detached boards are not supported on Windows yet — run `truthboard ui` in a terminal instead")

func supported() error { return errUnsupported }

func detachAttr() *syscall.SysProcAttr { return nil }

func Alive(pid int) bool { return false }

func terminate(pid int, grace time.Duration) error { return errUnsupported }
