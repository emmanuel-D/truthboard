package main

import "testing"

func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct {
		name         string
		stamped, mod string
		fromCheckout bool
		want         string
	}{
		// The release workflow's -X value is the truth whenever it exists,
		// even though a released binary also carries a module version.
		{"release stamp wins", "v0.8.3", "v0.8.3", false, "v0.8.3"},
		{"release stamp wins over a mismatched module", "v0.8.3", "v0.1.0", false, "v0.8.3"},

		// go install pkg@vX.Y.Z: no ldflags, but the toolchain records the
		// module version and there is no working copy behind it.
		{"go install names its release", "dev", "v0.8.3", false, "v0.8.3"},
		{"go install from a branch keeps its pseudo-version", "dev",
			"v0.0.0-20260720120000-abcdef123456", false, "v0.0.0-20260720120000-abcdef123456"},

		// A build from a checkout stays "dev" however convincing its module
		// version looks — this is what stops selfupdate replacing a working
		// copy. `go build` in a clone really does stamp these.
		{"dirty checkout build stays dev", "dev",
			"v0.8.4-0.20260720095844-439a3a04fae7+dirty", true, "dev"},
		{"clean checkout build stays dev", "dev",
			"v0.8.4-0.20260720095844-439a3a04fae7", true, "dev"},
		{"legacy devel marker stays dev", "dev", "(devel)", false, "dev"},
		{"absent build info stays dev", "dev", "", false, "dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveVersion(tc.stamped, tc.mod, tc.fromCheckout)
			if got != tc.want {
				t.Errorf("resolveVersion(%q, %q, %v) = %q, want %q",
					tc.stamped, tc.mod, tc.fromCheckout, got, tc.want)
			}
		})
	}
}
