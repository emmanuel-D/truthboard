package web

// Preflight proves the board can reach what it derives from, and says so
// once, before anything starts serving.
//
// The failure this exists to kill: a board deployed on a platform that
// restarts crashed containers hits a credential problem, git prints its own
// complaint, the container dies, the platform restarts it, and the operator
// reads the same wall of git output nine times over without ever learning
// which of their four environment variables is wrong. Git's own errors are
// no help here — GitLab answers a missing credential with the same "access
// denied" text it uses for a wrong one, so the reader hunts a bad token when
// none was supplied at all.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// deployDocs is where every preflight failure points. One place to read,
// rather than a different suggestion per error.
const deployDocs = "docs/deploy.md — \"private repositories, and multi-repo hubs\""

// Preflight runs the checks that must pass before a clone is attempted:
// that git's config-from-environment is coherent, and that the remote
// answers. It is the pre-clone half, so it takes a URL rather than a repo —
// on a fresh container there is no repo yet.
//
// Returning an error here is the point: the caller exits non-zero, and a
// restart loop then repeats a diagnosis instead of a stack of git errors.
func Preflight(remote string) error {
	if err := checkGitConfigEnv(os.LookupEnv); err != nil {
		return err
	}
	if remote == "" {
		return nil
	}
	return checkReachable(remote)
}

// PreflightRepo runs the checks that need a clone on disk: every spoke in
// the manifest, and — when intent editing is armed — whether this clone can
// actually push. These warn rather than fail. A hub whose spokes are
// temporarily unreachable still serves useful truth about the hub itself,
// and refusing to start would trade a degraded board for no board.
//
// Silent when everything is reachable: a healthy deploy gains no new noise.
func PreflightRepo(w io.Writer, repo string, o Options) {
	if ws, err := workspace.Load(repo); err == nil && ws != nil {
		var unreachable []string
		for _, r := range ws.Repos {
			if r.Remote == "" {
				continue // path-only spoke; nothing to reach
			}
			if err := checkReachable(r.Remote); err != nil {
				unreachable = append(unreachable, fmt.Sprintf("  %s: %v", r.Name, err))
			}
		}
		if len(unreachable) > 0 {
			fmt.Fprintf(w, "preflight: %d of %d spokes are unreachable — the board will report them as having no branches:\n%s\n",
				len(unreachable), len(ws.Repos), strings.Join(unreachable, "\n"))
		}
	}

	// An edit token arms writes that commit and push. Without push
	// credentials the board still starts, still serves, and still accepts
	// the edit — then fails on the push, so the first person to save a
	// story discovers the deploy is broken. Find out now instead.
	if o.Shared() && o.EditToken != "" {
		if err := checkPushable(repo); err != nil {
			fmt.Fprintf(w, "preflight: intent editing is armed but this clone cannot push — the first saved story will fail: %v\n", err)
		}
	}
}

// checkGitConfigEnv validates git's config-from-environment before any git
// command reads it. Git validates the whole GIT_CONFIG_COUNT set on every
// invocation, so one missing half takes down every git call with "missing
// config value" — an error that names the index but not what the index was
// for, and that surfaces wherever git happened to be called next.
func checkGitConfigEnv(lookup func(string) (string, bool)) error {
	raw, ok := lookup("GIT_CONFIG_COUNT")
	if !ok || raw == "" {
		return nil
	}
	count, err := strconv.Atoi(raw)
	if err != nil || count < 0 {
		return fmt.Errorf("GIT_CONFIG_COUNT is %q, which is not a count — git refuses every command while this is set; see %s", raw, deployDocs)
	}
	var missing []string
	for i := 0; i < count; i++ {
		key, hasKey := lookup(fmt.Sprintf("GIT_CONFIG_KEY_%d", i))
		_, hasValue := lookup(fmt.Sprintf("GIT_CONFIG_VALUE_%d", i))
		switch {
		case !hasKey && !hasValue:
			missing = append(missing, fmt.Sprintf("  %d: both GIT_CONFIG_KEY_%d and GIT_CONFIG_VALUE_%d are unset", i, i, i))
		case !hasKey:
			missing = append(missing, fmt.Sprintf("  %d: GIT_CONFIG_KEY_%d is unset", i, i))
		case !hasValue:
			missing = append(missing, fmt.Sprintf("  %d: GIT_CONFIG_VALUE_%d is unset (its key is %s)", i, i, gitrepo.Redact(key)))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("GIT_CONFIG_COUNT is %d, so git expects %d key/value pairs and %d %s incomplete:\n%s\nevery git command fails while this is so; see %s",
			count, count, len(missing), plural(len(missing), "is", "are"), strings.Join(missing, "\n"), deployDocs)
	}
	// The mirror image, and the quieter bug: a pair set but not counted is
	// silently ignored, so a credential that looks configured never applies.
	if _, ok := lookup(fmt.Sprintf("GIT_CONFIG_KEY_%d", count)); ok {
		return fmt.Errorf("GIT_CONFIG_KEY_%d is set but GIT_CONFIG_COUNT is %d, so git ignores it — the count must equal the number of pairs; see %s",
			count, count, deployDocs)
	}
	return nil
}

// checkReachable asks the remote for its refs. ls-remote is the cheapest
// operation that exercises the same read path a clone needs, and it neither
// writes nor keeps anything.
func checkReachable(remote string) error {
	cmd := exec.Command("git", "ls-remote", "--exit-code", "--heads", remote)
	// Without this git blocks forever on a credential prompt when no
	// terminal is attached, which on a container reads as a hang.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return explainRemote(remote, string(out))
}

// checkPushable asks the remote whether it would accept a push, without
// pushing. The receive-pack advertisement is permission-gated on every
// forge, so a read-only credential is refused here exactly as it would be
// on a real push.
func checkPushable(repo string) error {
	branch, ok := gitrepo.Try(repo, "symbolic-ref", "--short", "HEAD")
	if !ok {
		return fmt.Errorf("the clone is on a detached HEAD, so intent edits have no branch to push")
	}
	cmd := exec.Command("git", "-C", repo, "push", "--dry-run", "--quiet", "origin", branch)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	remote, _ := gitrepo.Try(repo, "remote", "get-url", "origin")
	return explainRemote(remote, string(out))
}

// explainRemote turns git's transport failure into the operator's actual
// problem. The classification matters most for credentials: forges answer a
// missing credential and a wrong one identically, so the reader cannot tell
// "I supplied nothing" from "what I supplied is wrong" — and those have
// opposite fixes.
func explainRemote(remote, out string) error {
	safe := gitrepo.Redact(remote)
	// git sanitizes most of its own output, and "most" is not a property
	// worth betting a token on — everything derived from out is redacted
	// before it is quoted back.
	out = gitrepo.Redact(out)
	low := strings.ToLower(out)
	switch {
	case containsAny(low, "authentication failed", "access denied", "403 forbidden", "401 unauthorized",
		"could not read username", "terminal prompts disabled", "invalid username or password",
		"the requested url returned error: 403", "the requested url returned error: 401"):
		// Forges name the missing permission when they can. Passing that
		// line through is worth more than anything this function could say.
		if detail := permissionLine(out); detail != "" {
			return fmt.Errorf("%s refused the credential: %s (see %s)", safe, detail, deployDocs)
		}
		if !hasCredential(remote) {
			// A forge hides a repository that does not exist behind the same
			// answer it gives a private one, so this cannot be narrowed
			// further from out here — say both rather than guess.
			return fmt.Errorf("%s needs a credential and none was supplied — or the URL names a repository that does not exist; a forge answers both the same way, so confirm the URL and that a credential is configured before suspecting the token you have; see %s", safe, deployDocs)
		}
		return fmt.Errorf("%s refused the credential — it is expired, or scoped without read access to this repository; see %s", safe, deployDocs)
	case containsAny(low, "not found", "404", "does not appear to be a git repository"):
		return fmt.Errorf("%s does not exist, or the credential cannot see it — a forge hides private repositories it will not serve; see %s", safe, deployDocs)
	case containsAny(low, "could not resolve host", "connection refused", "connection timed out", "network is unreachable"):
		return fmt.Errorf("%s is unreachable from here: %s", safe, firstLine(out))
	default:
		return fmt.Errorf("%s could not be read: %s", safe, firstLine(out))
	}
}

// permissionLine extracts a forge's own explanation, which is more specific
// than any guess: GitLab, for one, names the exact token permission missing
// ("requires a fine-grained personal access token with … [Code: Download]").
func permissionLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, found := strings.CutPrefix(line, "remote:"); found {
			rest = strings.TrimSpace(rest)
			if rest != "" && !strings.HasPrefix(strings.ToLower(rest), "http basic:") {
				return rest
			}
		}
	}
	return ""
}

// hasCredential reports whether the URL carries one itself or a rewrite rule
// could supply one. It cannot prove a rule matches this URL — git owns that
// resolution — so it only distinguishes "nothing is configured anywhere"
// from "something is, and was refused".
func hasCredential(remote string) bool {
	if gitrepo.Redact(remote) != remote {
		return true // the URL carries one itself
	}
	raw := os.Getenv("GIT_CONFIG_COUNT")
	return raw != "" && raw != "0"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return "no output"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
