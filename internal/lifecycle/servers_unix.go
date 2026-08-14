//go:build !windows

package lifecycle

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// mcpProcesses lists running `truthboard mcp` servers, newest field first:
// pid, elapsed time, then the command with its arguments.
//
// Elapsed time rather than start time: `ps` renders a start date in the
// caller's locale and time zone, and a check that exists to prevent a subtle
// wrong answer is the last place to parse a localised date. Elapsed is
// digits and colons in every locale.
//
// A machine with no usable `ps` reports nothing rather than failing: this is
// an advisory line under a status command, and it is never worth an error.
func mcpProcesses() []mcpProcess {
	out, err := exec.Command("ps", "-axo", "pid=,etime=,command=").Output()
	if err != nil {
		return nil
	}
	now := time.Now()
	self := os.Getpid()
	var found []mcpProcess
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// The command is everything after pid and etime; matching on the
		// joined fields collapses the padding ps uses to align columns.
		if !strings.Contains(strings.Join(fields[2:], " "), "truthboard mcp") {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == self {
			continue
		}
		elapsed, ok := parseElapsed(fields[1])
		if !ok {
			continue
		}
		found = append(found, mcpProcess{pid: pid, started: now.Add(-elapsed)})
	}
	return found
}

// parseElapsed reads ps's [[dd-]hh:]mm:ss elapsed format.
func parseElapsed(s string) (time.Duration, bool) {
	days := 0
	if d, rest, found := strings.Cut(s, "-"); found {
		n, err := strconv.Atoi(d)
		if err != nil || n < 0 {
			return 0, false
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	// Right-aligned: the last field is always seconds, the one before it
	// minutes, and hours only when ps printed them.
	units := []time.Duration{time.Second, time.Minute, time.Hour}
	total := time.Duration(days) * 24 * time.Hour
	for i := range parts {
		n, err := strconv.Atoi(parts[len(parts)-1-i])
		if err != nil || n < 0 {
			return 0, false
		}
		total += time.Duration(n) * units[i]
	}
	return total, true
}
