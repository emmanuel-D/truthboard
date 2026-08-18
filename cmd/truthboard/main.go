// Command truthboard audits a git repository: it derives work-unit statuses,
// a drift report, and a digest from repo reality — read-only, never asking a
// human for status.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/forge"
	"github.com/emmanuel-D/truthboard/internal/lifecycle"
	"github.com/emmanuel-D/truthboard/internal/mcp"
	"github.com/emmanuel-D/truthboard/internal/report"
	"github.com/emmanuel-D/truthboard/internal/selfupdate"
	"github.com/emmanuel-D/truthboard/internal/tui"
	"github.com/emmanuel-D/truthboard/internal/web"
)

// version is stamped by the release workflow via
// -ldflags "-X main.version=v0.x.y". The literal initialiser matters: -X only
// rewrites a variable initialised to a constant string, so this must never
// become a function call.
var version = "dev"

// init lets a `go install pkg@vX.Y.Z` build name its release. The toolchain
// applies no ldflags there, so without this the most likely install path for
// Go developers produces a binary calling itself a source build — and
// selfupdate refuses to replace those, quietly opting them out of updates.
func init() {
	mod, fromCheckout := "", false
	if bi, ok := debug.ReadBuildInfo(); ok {
		mod = bi.Main.Version
		for _, s := range bi.Settings {
			// Only a build from a working copy records its revision; a module
			// fetched from the proxy has no VCS settings at all.
			if s.Key == "vcs.revision" {
				fromCheckout = true
				break
			}
		}
	}
	version = resolveVersion(version, mod, fromCheckout)
}

// resolveVersion picks the version this binary reports. A release stamp always
// wins. Otherwise the module version the toolchain recorded stands in — but
// never for a build made from a checkout: `go build` in a clone stamps a
// VCS-derived pseudo-version like v0.8.4-0.2026…-439a3a04fae7+dirty, which
// looks like a release and is not one. Those stay "dev", which is what keeps
// selfupdate from overwriting a working copy someone is developing against.
func resolveVersion(stamped, module string, fromCheckout bool) string {
	if stamped != "dev" {
		return stamped
	}
	if fromCheckout {
		return stamped
	}
	if strings.HasPrefix(module, "v") {
		return module
	}
	return stamped
}

const usage = `truthboard — your repo already knows the status

Usage:
  truthboard audit [flags] [repo]           audit a repository (default: current directory)
                --status/--epic/--sprint/--since/--limit
                                            show less: narrow by derived status, epic, sprint,
                                            activity date or count. Narrowing changes what is
                                            shown, never what is derived
                --full                      json: every field of every story (the default
                                            summarises finished work to fit a context window)
  truthboard init [--agents [--hooks]] [repo]
                                            opt in to spec mode; --agents wires MCP +
                                            AGENTS.md so AI tools track work here by default
  truthboard init --workspace [api=git@host:acme/api.git …] [--path infra=../infra]
                                            scaffold a multi-repo hub: validated manifest,
                                            specs dir, and agent wiring (with multi-repo
                                            guidance) in one command; re-runs merge new
                                            spokes, never rewrite existing ones. With no
                                            pairs, the git repositories beside the hub are
                                            proposed as spokes — remotes read from their own
                                            configs — and declared only once you confirm
                --yes                       declare the proposed repositories without asking
                                            (non-interactive runs decline by default)
                --no-spokes                 wire only the hub — by default every spoke with a
                                            declared path: is wired too, its MCP server
                                            pointed back at the hub
                --commit                    commit the wiring in every repo it landed in
                --ui                        start the detached board when setup succeeds
  truthboard uninstall [--apply] [--specs] [repo]
                                            take the wiring back out: marker blocks, MCP
                                            registration, the commit-msg nudge, npm scripts
                                            and the run state under .git/. Prints the plan
                                            and writes nothing without --apply; your stories
                                            in .truthboard/ are kept unless --specs
  truthboard spec new "Title" [--owner X]   write intent once; status is derived from git
  truthboard brief <spec-id>                print the context packet for an agent or human
  truthboard next [repo]                    the highest-priority planned story, as a brief —
                                            deterministic, so "start the next story" is one call
  truthboard check <spec-id> <n|text|all>   tick the acceptance criteria that came true (the
                                            half of done git cannot derive); --uncheck reverts,
                                            no criterion prints the numbered checklist
  truthboard import <github|export.csv>     bring an existing backlog in: one story file per
                                            item, statuses left to git. --dry-run shows what it
                                            would write; closed items are skipped by default
  truthboard mirror [--apply]               publish the board as issues on the repo's forge, for
                                            the people who never open a terminal. Shows the plan
                                            and writes nothing unless --apply; each issue says it
                                            is a mirror and names the story it came from
  truthboard since <ref|commit|date>        what changed on the board since then: landed, filed,
                                            reverted, signed off — derived from two commits, so
                                            nothing had to be running and no state is kept
  truthboard find "text" [--limit n]        has this already been filed? searches ids, titles,
                                            epics and story text — one cheap answer, not the board
  truthboard link <spec-id> <branch-glob>   fix a linking miss (fixes the input, not the status)
  truthboard mcp [repo]                     serve specs/board over MCP (stdio) for AI agents;
                                            repo defaults to the current directory, which the
                                            MCP client picks — pass a path when they differ
  truthboard board [repo]                   the board in your terminal (read-only TUI):
                                            kanban columns, drift, digest — keyboard only
  truthboard draft "Concept" [--owner X]    LLM drafts an epic of real stories (goal +
                                            acceptance) — needs ANTHROPIC_API_KEY or OLLAMA_HOST
  truthboard summary [sprint] [--ids]       what was delivered, what is paused and why —
                                            plain language, no jargon, no ids, no API key
  truthboard plan [sprint]                  LLM narrates the sprint about to start: rollover,
                                            ready vs blocked candidates, committed points against
                                            what the last sprint landed (sprint defaults to the
                                            next dated one); the same facts sit in audit --format json
  truthboard review [sprint]                LLM narrates a sprint review from derived facts
  truthboard ui [--port 1337] [--forge] [--no-open] [--detach] [repo]
                                            web board; --detach keeps it running in the background
                --fetch 60s                 poll origin so the board tracks the remote, not just
                                            this clone (fast-forwards only a clean checkout)
                --host 0.0.0.0              share the board beyond this machine (read-only)
                --notify <url>              post stalled/regressed transitions to a webhook
                                            (Slack-compatible; recoveries are news too)
                --digest 24h                also post a what-changed digest on this interval:
                                            landed, signed off, and landed work whose acceptance
                                            is still unread. Silent when nothing changed
  truthboard preflight [--remote URL] [repo]
                                            prove a deploy can reach what it derives from:
                                            git's environment, the remote, every spoke, and
                                            push access when editing is armed
  truthboard status [repo]                  is a detached board running for this repo?
  truthboard stop [repo]                    stop the detached board
  truthboard update [--check]               update this binary to the latest release
                                            (detached boards need a stop/detach after)
  truthboard version

Every command takes -h for its flags (e.g. truthboard audit -h).

Getting started in an existing project:
  cd your-project
  truthboard init --agents --hooks    specs + MCP + AGENTS.md + trailer nudge
  truthboard ui --detach              the board, running in the background

  Then write a story (truthboard spec new "Title"), work on a branch
  containing its id, end commits with "Spec: <id>" — the board does the rest.
  npm projects also get: npm run board / board:status / board:stop / board:audit
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "audit":
		os.Exit(runAudit(os.Args[2:]))
	case "init":
		os.Exit(runInit(os.Args[2:]))
	case "uninstall":
		os.Exit(runUninstall(os.Args[2:]))
	case "spec":
		os.Exit(runSpec(os.Args[2:]))
	case "brief":
		os.Exit(runBrief(os.Args[2:]))
	case "next":
		os.Exit(runNext(os.Args[2:]))
	case "check":
		os.Exit(runCheck(os.Args[2:]))
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "find":
		os.Exit(runFind(os.Args[2:]))
	case "since":
		os.Exit(runSince(os.Args[2:]))
	case "mirror":
		os.Exit(runMirror(os.Args[2:]))
	case "import":
		os.Exit(runImport(os.Args[2:]))
	case "mcp":
		os.Exit(runMcp(os.Args[2:], os.Stdin, os.Stdout))
	case "board":
		os.Exit(runBoard(os.Args[2:]))
	case "draft":
		os.Exit(runDraft(os.Args[2:]))
	case "summary":
		os.Exit(runSummary(os.Args[2:]))
	case "plan":
		os.Exit(runPlan(os.Args[2:]))
	case "review":
		os.Exit(runReview(os.Args[2:]))
	case "ui":
		os.Exit(runUI(os.Args[2:]))
	case "preflight":
		os.Exit(runPreflight(os.Args[2:]))
	case "status":
		os.Exit(runLifecycle("status", func(repo string) (string, error) {
			line, err := lifecycle.Status(repo, version)
			if err != nil {
				return line, err
			}
			// A detached board and an MCP server go stale the same way and
			// for the same reason — a process outliving the release that
			// started it. One command answers for both, or the one nobody
			// thought to check is the one serving old truth.
			for _, s := range lifecycle.StaleServers() {
				line += "\n" + s
			}
			return line, nil
		}, os.Args[2:]))
	case "stop":
		os.Exit(runLifecycle("stop", lifecycle.Stop, os.Args[2:]))
	case "update":
		os.Exit(runUpdate(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("truthboard " + version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

// runMcp serves one repository over MCP. The repo argument matters more
// here than anywhere else: every other command runs in a directory the
// person chose, while an MCP server runs in whatever directory the client
// spawned it from — which, for a hub living in a subdirectory, is usually
// the wrong one. in/out are parameters so a test can drive a real session.
func runMcp(args []string, in io.Reader, out io.Writer) int {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), `usage: truthboard mcp [repo]

Serve specs and the derived board over MCP (stdio) for AI agents.

  repo   repository to serve (default: the current directory — which the
         MCP client picks, not you, so pass a path when they differ)
`)
	}
	fs.Parse(args)
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	if err := mcp.Serve(in, out, repo, version); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard mcp: %v\n", err)
		return 1
	}
	return 0
}

func runBoard(args []string) int {
	fs := flag.NewFlagSet("board", flag.ExitOnError)
	fs.Parse(args)
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	if err := tui.Run(repo); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard board: %v\n", err)
		return 1
	}
	return 0
}

func runAudit(args []string) int {
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	staleDays := fs.Int("stale-days", 7, "days without commits before a branch counts as stalled")
	digestDays := fs.Int("digest-days", 14, "window for the digest and shadow-work scan")
	flowDays := fs.Int("flow-days", 90, "window for cycle time, throughput and work in flight")
	format := fs.String("format", "term", "output format: term, md, json")
	status := fs.String("status", "", "show only these derived statuses (comma-separated: planned,in-progress,in-review,stalled,done,regressed)")
	epic := fs.String("epic", "", "show only stories in this epic")
	sprint := fs.String("sprint", "", "show only stories in this sprint")
	since := fs.String("since", "", "show only stories with a commit on or after this date (2026-08-01)")
	limit := fs.Int("limit", 0, "show at most this many stories, in backlog order")
	full := fs.Bool("full", false, "json: carry every field of every story and branch (default summarises finished work)")
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	noForge := fs.Bool("no-forge", false, "skip tracker enrichment")
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	// The filter is parsed before the audit runs: a typo in --status should
	// cost nothing and say so, not sit behind twenty seconds of git.
	filter, err := audit.ParseFilter(splitList(*status), *epic, *sprint, *since, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 2
	}

	opts := audit.Options{StaleDays: *staleDays, DigestDays: *digestDays, FlowDays: *flowDays}
	res, err := audit.Audit(repo, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if !*noForge {
		audit.EnrichWithForges(res, forge.Fetch, opts)
	}

	res = res.Filtered(filter)

	switch *format {
	case "term":
		err = report.Terminal(os.Stdout, res, !*noColor && isTTY())
	case "md":
		err = report.Markdown(os.Stdout, res)
	case "json":
		// Summarising is a wire-format concern: the terminal and the report
		// are read by someone who can scroll, the JSON by something with a
		// context window. Same statuses either way.
		if !*full {
			res = res.Lean()
		}
		err = report.JSON(os.Stdout, res)
	default:
		fmt.Fprintf(os.Stderr, "truthboard: unknown format %q (want term, md, or json)\n", *format)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	return 0
}

func runUI(args []string) int {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	port := fs.Int("port", 1337, "port to listen on")
	host := fs.String("host", "", "listen host (default loopback; beyond loopback the board serves read-only)")
	useForge := fs.Bool("forge", false, "enrich the board with tracker data (slower refresh)")
	fetch := fs.Duration("fetch", 0, "poll origin on this interval (e.g. 60s) so the board tracks the remote")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	detach := fs.Bool("detach", false, "run the board in the background (truthboard status / stop to manage)")
	webhookSecret := fs.String("webhook-secret", os.Getenv("TRUTHBOARD_WEBHOOK_SECRET"),
		"arm POST /webhook: a forge push webhook with this secret triggers an immediate fetch (env TRUTHBOARD_WEBHOOK_SECRET)")
	notify := fs.String("notify", os.Getenv("TRUTHBOARD_NOTIFY_URL"),
		"post stalled/regressed transitions to this webhook URL (Slack-compatible JSON; env TRUTHBOARD_NOTIFY_URL)")
	digest := fs.Duration("digest", 0,
		"post a what-changed digest to --notify on this interval (e.g. 24h): landed, signed off, and landed work whose acceptance is still unread")
	editToken := fs.String("edit-token", os.Getenv("TRUTHBOARD_EDIT_TOKEN"),
		"arm intent editing on a shared board: requests carrying this token may create/edit stories, and each edit is committed and pushed to origin (env TRUTHBOARD_EDIT_TOKEN)")
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	opts := web.Options{Port: *port, Host: *host, Forge: *useForge,
		FetchEvery: *fetch, OpenBrowser: !*noOpen, Version: version,
		WebhookSecret: *webhookSecret, NotifyURL: *notify, EditToken: *editToken,
		DigestEvery: *digest}
	if *detach {
		state, err := lifecycle.Detach(repo, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 1
		}
		fmt.Printf("board running in the background · %s · pid %d\n", state.URL, state.PID)
		fmt.Println("  truthboard status   check on it\n  truthboard stop     stop it")
		if !*noOpen {
			web.Browse(state.URL)
		}
		return 0
	}
	if err := web.Serve(repo, opts); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	return 0
}

// runPreflight is what a container runs before it clones anything: it
// proves git's environment is coherent and the remote answers, so a deploy
// that is going to fail says why once, in the operator's terms, instead of
// failing later as raw git output on every restart.
func runPreflight(args []string) int {
	fs := flag.NewFlagSet("preflight", flag.ExitOnError)
	remote := fs.String("remote", os.Getenv("REPO_URL"),
		"check this remote is readable (env REPO_URL); omit to check only git's environment")
	fs.Parse(args)

	if err := web.Preflight(*remote); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard preflight: %v\n", err)
		return 1
	}
	// The repo half only applies where there is one; a pre-clone run has
	// nothing on disk yet and skips straight to a silent pass.
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		web.PreflightRepo(os.Stderr, repo, web.Options{
			Host: os.Getenv("HOST"), EditToken: os.Getenv("TRUTHBOARD_EDIT_TOKEN")})
	}
	return 0
}

func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ExitOnError)
	check := fs.Bool("check", false, "only report current vs latest; change nothing")
	fs.Parse(args)
	if err := selfupdate.Run(os.Stdout, version, *check); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	return 0
}

func runLifecycle(name string, op func(string) (string, error), args []string) int {
	repo := "."
	if len(args) > 0 {
		if args[0] == "-h" || args[0] == "--help" {
			fmt.Printf("usage: truthboard %s [repo]   (repo defaults to the current directory)\n", name)
			return 0
		}
		repo = args[0]
	}
	msg, err := op(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Println(msg)
	return 0
}

func isTTY() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
