package lifecycle

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// StaleServers reports MCP server processes on this machine that started
// before the truthboard now installed — the same staleness Status already
// reports for a detached board, for the process agents actually read the
// board through.
//
// A client spawns `truthboard mcp` once and keeps it for the session, so an
// upgrade never reaches a server already running: it keeps deriving statuses
// by the rules of the build it started with, and nothing in its answers looks
// wrong. The server warns about itself too, in every status-bearing result,
// but only an agent talking to it ever sees that. This is the answer to the
// question a human asks in a terminal — what on this machine is serving old
// truth? — next to the boards it already answers for.
//
// Start time against the installed binary's mtime, rather than asking each
// process its version: an MCP server speaks JSON-RPC on the stdin its client
// holds, so there is nobody to ask. A process older than the file it would
// have been launched from cannot be running that file's code.
func StaleServers() []string {
	installed, at, ok := installedBinary()
	if !ok {
		return nil
	}
	var stale []string
	for _, p := range mcpProcesses() {
		if !p.started.Before(at) {
			continue
		}
		stale = append(stale, fmt.Sprintf(
			"⚠ an MCP server (pid %d) started %s, before the truthboard at\n"+
				"  %s was installed %s. It serves the\n"+
				"  code it started with — restart the client that spawned it.",
			p.pid, p.started.Format("Jan 2 15:04"), installed, at.Format("Jan 2 15:04")))
	}
	return stale
}

// mcpProcess is one running `truthboard mcp`.
type mcpProcess struct {
	pid     int
	started time.Time
}

// installedBinary locates the truthboard on PATH and when it was installed.
// Anything unanswerable — no truthboard, an unreadable one — reports false
// and says nothing, exactly like the MCP server's own check.
func installedBinary() (path string, installed time.Time, ok bool) {
	bin, err := exec.LookPath("truthboard")
	if err != nil {
		return "", time.Time{}, false
	}
	info, err := os.Stat(bin)
	if err != nil {
		return "", time.Time{}, false
	}
	return bin, info.ModTime(), true
}
