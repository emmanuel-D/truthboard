// Command truthboard audits a git repository: it derives work-unit statuses,
// a drift report, and a digest from repo reality — read-only, never asking a
// human for status.
package main

import (
	"flag"
	"fmt"
	"os"

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
// -ldflags "-X main.version=v0.x.y"; dev builds show the fallback.
var version = "dev"

const usage = `truthboard — your repo already knows the status

Usage:
  truthboard audit [flags] [repo]           audit a repository (default: current directory)
  truthboard init [--agents [--hooks]] [repo]
                                            opt in to spec mode; --agents wires MCP +
                                            AGENTS.md so AI tools track work here by default
  truthboard spec new "Title" [--owner X]   write intent once; status is derived from git
  truthboard brief <spec-id>                print the context packet for an agent or human
  truthboard next [repo]                    the highest-priority planned story, as a brief —
                                            deterministic, so "start the next story" is one call
  truthboard link <spec-id> <branch-glob>   fix a linking miss (fixes the input, not the status)
  truthboard mcp                            serve specs/board over MCP (stdio) for AI agents
  truthboard board [repo]                   the board in your terminal (read-only TUI):
                                            kanban columns, drift, digest — keyboard only
  truthboard ui [--port 1337] [--forge] [--no-open] [--detach] [repo]
                                            web board; --detach keeps it running in the background
                --fetch 60s                 poll origin so the board tracks the remote, not just
                                            this clone (fast-forwards only a clean checkout)
                --host 0.0.0.0              share the board beyond this machine (read-only)
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
	case "spec":
		os.Exit(runSpec(os.Args[2:]))
	case "brief":
		os.Exit(runBrief(os.Args[2:]))
	case "next":
		os.Exit(runNext(os.Args[2:]))
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "mcp":
		if err := mcp.Serve(os.Stdin, os.Stdout, ".", version); err != nil {
			fmt.Fprintf(os.Stderr, "truthboard mcp: %v\n", err)
			os.Exit(1)
		}
	case "board":
		os.Exit(runBoard(os.Args[2:]))
	case "ui":
		os.Exit(runUI(os.Args[2:]))
	case "status":
		os.Exit(runLifecycle("status", lifecycle.Status, os.Args[2:]))
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
	format := fs.String("format", "term", "output format: term, md, json")
	noColor := fs.Bool("no-color", false, "disable ANSI colors")
	noForge := fs.Bool("no-forge", false, "skip tracker enrichment")
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	opts := audit.Options{StaleDays: *staleDays, DigestDays: *digestDays}
	res, err := audit.Audit(repo, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if !*noForge {
		if data, ok := forge.Fetch(repo); ok {
			audit.EnrichWithForge(res, data, opts)
		}
	}

	switch *format {
	case "term":
		err = report.Terminal(os.Stdout, res, !*noColor && isTTY())
	case "md":
		err = report.Markdown(os.Stdout, res)
	case "json":
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
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	opts := web.Options{Port: *port, Host: *host, Forge: *useForge,
		FetchEvery: *fetch, OpenBrowser: !*noOpen, Version: version}
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
