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
	"github.com/emmanuel-D/truthboard/internal/mcp"
	"github.com/emmanuel-D/truthboard/internal/report"
	"github.com/emmanuel-D/truthboard/internal/web"
)

const version = "0.1.0"

const usage = `truthboard — your repo already knows the status

Usage:
  truthboard audit [flags] [repo]           audit a repository (default: current directory)
  truthboard init [repo]                    opt in to spec mode (.truthboard/specs/)
  truthboard spec new "Title" [--owner X]   write intent once; status is derived from git
  truthboard brief <spec-id>                print the context packet for an agent or human
  truthboard link <spec-id> <branch-glob>   fix a linking miss (fixes the input, not the status)
  truthboard mcp                            serve specs/board over MCP (stdio) for AI agents
  truthboard ui [--port 1337] [--forge] [--no-open] [repo]
                                            read-only web board (for PMs/POs)
  truthboard version

Flags for audit:
  --stale-days N    days without commits before a branch counts as stalled (default 7)
  --digest-days N   window for the digest and shadow-work scan (default 14)
  --format F        output format: term, md, json (default term)
  --no-color        disable ANSI colors in term output
  --no-forge        skip tracker enrichment (GitHub issues/PRs via gh)
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
	case "link":
		os.Exit(runLink(os.Args[2:]))
	case "mcp":
		if err := mcp.Serve(os.Stdin, os.Stdout, ".", version); err != nil {
			fmt.Fprintf(os.Stderr, "truthboard mcp: %v\n", err)
			os.Exit(1)
		}
	case "ui":
		os.Exit(runUI(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("truthboard " + version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
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
	port := fs.Int("port", 1337, "port to listen on (localhost only)")
	useForge := fs.Bool("forge", false, "enrich the board with tracker data (slower refresh)")
	noOpen := fs.Bool("no-open", false, "do not open the browser")
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	if err := web.Serve(repo, *port, *useForge, !*noOpen, version); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	return 0
}

func isTTY() bool {
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
