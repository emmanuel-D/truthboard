//go:build !windows

package lifecycle

import (
	"testing"
	"time"
)

// ps renders elapsed time as [[dd-]hh:]mm:ss. Reading it wrong would age a
// current server into a stale one — a warning telling someone to restart a
// process that is already right.
func TestParseElapsedReadsEveryPsShape(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"00:07", 7 * time.Second, true},
		{"12:34", 12*time.Minute + 34*time.Second, true},
		{"01:02:03", time.Hour + 2*time.Minute + 3*time.Second, true},
		{"17-08:30:00", 17*24*time.Hour + 8*time.Hour + 30*time.Minute, true},
		{"2-00:00:01", 2*24*time.Hour + time.Second, true},
		{"", 0, false},
		{"7", 0, false},
		{"1:2:3:4", 0, false},
		{"aa:bb", 0, false},
		{"x-01:00:00", 0, false},
		{"-1:00", 0, false},
	} {
		got, ok := parseElapsed(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseElapsed(%q) = %v, %v — want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// The list is advisory, under a status line. On a machine with no truthboard
// on PATH there is nothing to compare against, and it says so by saying
// nothing — never by failing the command someone ran to get their bearings.
func TestStaleServersNeverFails(t *testing.T) {
	for _, s := range StaleServers() {
		if s == "" {
			t.Error("an empty finding is a finding nobody can act on")
		}
	}
}
