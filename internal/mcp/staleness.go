package mcp

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A client spawns `truthboard mcp` once and keeps the process for the life of
// the session, so an upgrade underneath it leaves the agent talking to the
// code the server started with. Nothing about that looks wrong from the
// agent's side: it asks for the board and gets a confident, well-formed,
// superseded answer.
//
// It has already cost real work. On 2026-08-14, with v0.17.0 installed, a
// session's get_board reported a story that had been filed and never started
// as done — "work landed on main", listed under shipped — and next_spec
// answered "nothing is startable". The same repo read by the installed binary
// at the same moment said planned, and `truthboard next` handed the story
// over. An agent that believes "nothing is startable" ends the session.
//
// `truthboard status` already flags a detached board older than your binary
// for exactly this reason. This is that check, for the process agents
// actually read the board through — and it warns, never refuses: a stale
// server is mostly right, and one that stopped answering would strand every
// session that reached it.
type staleness struct {
	once sync.Once
	// probe answers "what is installed?"; nil means ask PATH. A seam, so the
	// once-per-lifetime promise is testable without a second truthboard.
	probe  func() string
	banner string
}

// warning returns the banner to attach to a status-bearing answer, or "" when
// there is nothing to say. The probe costs a process spawn, so it runs once
// per server lifetime — staleness cannot appear mid-session anyway, since
// this process's own build never changes.
func (s *staleness) warning(serving string) string {
	s.once.Do(func() {
		probe := s.probe
		if probe == nil {
			probe = installedVersion
		}
		s.banner = staleBanner(serving, probe())
	})
	return s.banner
}

// staleBanner is the whole decision as a pure function of two version
// strings, so the thing that must never cry wolf is testable without a PATH,
// a filesystem, or a second build of truthboard.
func staleBanner(serving, installed string) string {
	mine, ok := release(serving)
	theirs, okInstalled := release(installed)
	if !ok || !okInstalled || !older(mine, theirs) {
		return ""
	}
	return fmt.Sprintf(
		"⚠ this MCP server is truthboard %s, but %s is installed on this machine.\n"+
			"  A client spawns the server once and keeps the process, so it serves the\n"+
			"  code it started with — statuses in this answer may be derived by rules a\n"+
			"  later release has already corrected. Restart your MCP client to respawn\n"+
			"  the server. Until you do, cross-check anything you are about to act on:\n"+
			"    truthboard audit     # and `truthboard next` for the startable story",
		serving, installed)
}

// release parses a clean release tag — vX.Y.Z and nothing else.
//
// Anything else is unanswerable rather than old: "dev" is what a build from a
// checkout reports (resolveVersion keeps it that way), and a pre-release or
// build suffix means someone is deliberately running something other than a
// release. Warning either of them would be nagging a developer about their
// own build, which is how a warning becomes noise people skim past — the one
// failure this check cannot afford, since the whole point is to be believed
// the once it fires.
func release(v string) ([3]int, bool) {
	var out [3]int
	digits, ok := strings.CutPrefix(strings.TrimSpace(v), "v")
	if !ok {
		return out, false
	}
	parts := strings.Split(digits, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// older reports whether a precedes b.
func older(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// installedVersion asks the truthboard on PATH which build it is. Every
// failure answers "" — no truthboard, an unreadable one, one that hangs —
// because an unanswerable comparison is never a warning and never an error.
// The server must keep answering tool calls whatever this returns.
func installedVersion() string {
	bin, err := exec.LookPath("truthboard")
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return ""
	}
	// `truthboard version` prints "truthboard v0.17.0".
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// statusBearing names the tools whose answer is a derived status — the ones
// an agent acts on, and so the ones a superseded rule can mislead. The
// intent-writing tools (create_spec, update_spec, check_acceptance) write
// what they were told and are not banner-worthy: repeating the warning on
// every call is how it stops being read.
var statusBearing = map[string]bool{
	"get_board":  true,
	"next_spec":  true,
	"list_specs": true,
	"get_brief":  true,
}
