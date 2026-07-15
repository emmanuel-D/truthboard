package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

func runInit(args []string) int {
	repo := "."
	if len(args) > 0 {
		repo = args[0]
	}
	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Printf("initialized %s\n\nNext:\n", dir)
	fmt.Println(`  truthboard spec new "Your first unit of work"   write intent once`)
	fmt.Println("  truthboard audit                                 everything else is derived")
	return 0
}

func runSpec(args []string) int {
	if len(args) < 1 || args[0] != "new" {
		fmt.Fprintln(os.Stderr, `usage: truthboard spec new "Title" [--owner name] [--repo path]`)
		return 2
	}
	fs := flag.NewFlagSet("spec new", flag.ExitOnError)
	owner := fs.String("owner", "", "who owns this spec")
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
		fmt.Fprintln(os.Stderr, `usage: truthboard spec new "Title" [--owner name]`)
		return 2
	}

	s, err := spec.New(*repo, title, *owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
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

	s, err := spec.Find(*repo, id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}

	// The context packet an agent (or human) needs to start working.
	fmt.Printf("Analyze and resolve spec %s in this repository.\n\n---\n", s.ID)
	fmt.Printf("Title: %s\n", s.Title)
	if s.Owner != "" {
		fmt.Printf("Owner: %s\n", s.Owner)
	}
	if len(s.Paths) > 0 {
		fmt.Printf("Scope: %s\n", strings.Join(s.Paths, ", "))
	}
	fmt.Printf("\n%s\n---\n\n", s.Body)
	fmt.Printf("Work on a branch matching %q (or any branch containing %q).\n", s.Branch, s.ID)
	fmt.Printf("End every commit message with the trailer:\n\n    %s\n\n", s.Trailer())
	fmt.Println("Satisfy the acceptance criteria while maintaining code health.")

	if res, err := audit.Audit(*repo, audit.Options{}); err == nil {
		for _, ss := range res.Specs {
			if ss.ID == s.ID && ss.Status != audit.Planned {
				fmt.Printf("Current derived status: %s (%s)\n", ss.Status, ss.Evidence)
			}
		}
	}
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
