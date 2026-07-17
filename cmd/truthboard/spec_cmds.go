package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/adopt"
	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	agents := fs.Bool("agents", false, "wire the repo for AI agents: MCP registration + AGENTS.md working agreement")
	hooks := fs.Bool("hooks", false, "with --agents: install a commit-msg hook that warns (never blocks) on missing Spec trailers")
	fs.Parse(args)
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Printf("initialized %s\n", dir)

	// Ecosystem detection: npm projects get the lifecycle as npm scripts.
	npmLog, err := adopt.NpmScripts(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	for _, line := range npmLog {
		fmt.Println("  " + line)
	}

	if *agents {
		log, err := adopt.Agents(repo, *hooks)
		if err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 1
		}
		for _, line := range log {
			fmt.Println("  " + line)
		}
	}

	fmt.Println("\nNext:")
	fmt.Println(`  truthboard spec new "Your first unit of work"   write intent once`)
	fmt.Println("  truthboard audit                                 everything else is derived")
	return 0
}

func runSpec(args []string) int {
	if len(args) < 1 || args[0] != "new" {
		fmt.Fprintln(os.Stderr, `usage: truthboard spec new "Title" [--owner name] [--sprint slug] [--repo path]`)
		return 2
	}
	fs := flag.NewFlagSet("spec new", flag.ExitOnError)
	owner := fs.String("owner", "", "who owns this spec")
	sprint := fs.String("sprint", "", "iteration slug (e.g. s12) — intent, never a status")
	repo := fs.String("repo", ".", "repository path")
	// stdlib flag stops at the first positional arg, so split the title
	// (everything before the first flag) from the flags ourselves.
	rest := args[1:]
	var titleParts []string
	for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		titleParts = append(titleParts, rest[0])
		rest = rest[1:]
	}
	fs.Parse(rest)
	title := strings.Join(titleParts, " ")
	if title == "" {
		fmt.Fprintln(os.Stderr, `usage: truthboard spec new "Title" [--owner name] [--sprint slug]`)
		return 2
	}

	s, err := spec.New(*repo, title, *owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if *sprint != "" {
		s.Sprint = *sprint
		if err := s.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 1
		}
	}
	fmt.Printf("created %s\n\n", s.File)
	fmt.Printf("  id:      %s\n  branch:  %s (suggested glob — any branch containing %q links too)\n  trailer: %s (add to commits for the strongest link)\n",
		s.ID, s.Branch, s.ID, s.Trailer())
	fmt.Printf("\nEdit the Goal and Acceptance sections, then: truthboard brief %s\n", s.ID)
	return 0
}

func runBrief(args []string) int {
	fs := flag.NewFlagSet("brief", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: truthboard brief <spec-id>")
		return 2
	}
	id := fs.Arg(0)

	text, err := audit.Brief(*repo, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Print(text)
	return 0
}

func runNext(args []string) int {
	fs := flag.NewFlagSet("next", flag.ExitOnError)
	fs.Parse(args)
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	next, stalled, err := audit.Next(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if next == nil {
		msg := "nothing is planned — every story has work in flight or landed."
		if stalled > 0 {
			msg += fmt.Sprintf(" %d stalled — worth resuming? See truthboard audit.", stalled)
		}
		msg += ` New intent: truthboard spec new "Title"`
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}

	pri := ""
	if next.Priority > 0 {
		pri = fmt.Sprintf(" (priority %d)", next.Priority)
	}
	fmt.Printf("next up: %s — %s%s\n\n", next.ID, next.Title, pri)
	text, err := audit.Brief(repo, next.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Print(text)
	return 0
}

func runLink(args []string) int {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	fs.Parse(args)
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: truthboard link <spec-id> <branch-or-glob>")
		return 2
	}
	id, branch := fs.Arg(0), fs.Arg(1)

	s, err := spec.Find(*repo, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	s.Branch = branch
	if err := s.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Printf("linked %s to branch %q — the fix went into the spec file, the status stays derived\n", s.ID, branch)
	return 0
}
