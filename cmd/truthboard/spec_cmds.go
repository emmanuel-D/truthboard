package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/adopt"
	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// repeatedFlag collects a flag given multiple times (--path a=x --path b=y).
type repeatedFlag []string

func (r *repeatedFlag) String() string { return strings.Join(*r, ", ") }
func (r *repeatedFlag) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	agents := fs.Bool("agents", false, "wire the repo for AI agents: MCP registration + AGENTS.md working agreement")
	hooks := fs.Bool("hooks", false, "with --agents: install a commit-msg hook that warns (never blocks) on missing Spec trailers")
	wsFlag := fs.Bool("workspace", false, "scaffold a multi-repo hub: name=remote pairs become .truthboard/workspace.yml (implies --agents)")
	var pathFlags repeatedFlag
	fs.Var(&pathFlags, "path", "with --workspace: name=dir declares a local checkout (repeatable; alone or alongside a name=remote pair)")

	// stdlib flag stops at the first positional (the name=remote pairs),
	// so lift flag tokens out first; only --path takes a value.
	var flagArgs, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		flagArgs = append(flagArgs, a)
		if (a == "--path" || a == "-path") && i+1 < len(args) {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}
	fs.Parse(flagArgs)

	repo := "."
	var pairs []string
	for _, p := range pos {
		if strings.Contains(p, "=") {
			pairs = append(pairs, p)
			continue
		}
		repo = p
	}
	if (len(pairs) > 0 || len(pathFlags) > 0) && !*wsFlag {
		fmt.Fprintln(os.Stderr, "truthboard: name=remote pairs and --path need --workspace")
		return 2
	}
	spokes, err := parseSpokes(pairs, pathFlags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 2
	}
	if *wsFlag {
		// A workspace hub is an agent hub: the wiring below then includes
		// the multi-repo decomposition guidance because the manifest exists.
		*agents = true
	}

	dir := spec.Dir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	fmt.Printf("initialized %s\n", dir)

	if *wsFlag {
		log, err := workspace.Declare(repo, spokes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 2
		}
		fmt.Printf("workspace manifest: %s\n", workspace.File)
		for _, line := range log {
			fmt.Println("  " + line)
		}
		fmt.Println("  note: the audit reads spokes from declared paths or the board server's")
		fmt.Println("  clones — until one exists, each spoke shows on the board as a loud")
		fmt.Println("  unreadable finding (expected, not broken). truthboard ui --detach clones them.")
	}

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

	// The git check comes last: the wiring above is correct on disk either
	// way, but nothing it writes derives a status until a repository exists.
	needsRepo := adopt.RepoWarning(repo)
	for _, line := range needsRepo {
		fmt.Println("  " + line)
	}

	fmt.Println("\nNext:")
	if needsRepo != nil {
		fmt.Println("  git init                                       truthboard reads git, so start there")
	}
	fmt.Println(`  truthboard spec new "Your first unit of work"   write intent once`)
	fmt.Println("  truthboard audit                                 everything else is derived")
	return 0
}

// parseSpokes turns name=remote pairs and --path name=dir flags into spoke
// declarations. A --path may annotate a declared pair (a local checkout for
// a remote) or stand alone (a path-only spoke).
func parseSpokes(pairs, paths []string) ([]workspace.Repo, error) {
	var spokes []workspace.Repo
	idx := map[string]int{}
	for _, p := range pairs {
		name, remote, _ := strings.Cut(p, "=")
		if name == "" || remote == "" {
			return nil, fmt.Errorf("%q — want name=remote", p)
		}
		if _, dup := idx[name]; dup {
			return nil, fmt.Errorf("repo %q declared twice", name)
		}
		idx[name] = len(spokes)
		spokes = append(spokes, workspace.Repo{Name: name, Remote: remote})
	}
	for _, p := range paths {
		name, dir, ok := strings.Cut(p, "=")
		if !ok || name == "" || dir == "" {
			return nil, fmt.Errorf("--path %q — want name=dir", p)
		}
		if i, ok := idx[name]; ok {
			if spokes[i].Path != "" {
				return nil, fmt.Errorf("--path for %q given twice", name)
			}
			spokes[i].Path = dir
			continue
		}
		idx[name] = len(spokes)
		spokes = append(spokes, workspace.Repo{Name: name, Path: dir})
	}
	return spokes, nil
}

func runSpec(args []string) int {
	if len(args) < 1 || args[0] != "new" {
		fmt.Fprintln(os.Stderr, `usage: truthboard spec new "Title" [--owner name] [--sprint slug] [--points n] [--type story|bug|task] [--repo path]`)
		return 2
	}
	fs := flag.NewFlagSet("spec new", flag.ExitOnError)
	owner := fs.String("owner", "", "who owns this spec")
	sprint := fs.String("sprint", "", "iteration slug (e.g. s12) — intent, never a status")
	points := fs.Int("points", 0, "story-point estimate; 0 = unestimated")
	typ := fs.String("type", "", "story | bug | task (default story)")
	needsFlag := fs.String("needs", "", "comma-separated spec ids that must land first (e.g. tb-1a2b,tb-3c4d)")
	reposFlag := fs.String("repos", "", "comma-separated workspace repos this story must land in (\"hub\" or spoke names); done requires all of them")
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

	// Validate every intent argument before creating the file, so a typo
	// never leaves an orphan spec behind.
	if !spec.ValidType(*typ) {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", spec.ErrType(*typ))
		return 2
	}
	var needs []string
	if *needsFlag != "" {
		for _, id := range strings.Split(*needsFlag, ",") {
			needs = append(needs, strings.TrimSpace(id))
		}
		if err := spec.ValidateNeeds(*repo, needs, ""); err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 2
		}
	}
	var repos []string
	if *reposFlag != "" {
		for _, r := range strings.Split(*reposFlag, ",") {
			repos = append(repos, strings.TrimSpace(r))
		}
		if err := spec.ValidateRepos(*repo, repos); err != nil {
			fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
			return 2
		}
	}
	s, err := spec.New(*repo, title, *owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if *sprint != "" || *points > 0 || *typ != "" || len(needs) > 0 || len(repos) > 0 {
		s.Sprint = *sprint
		s.Points = *points
		s.Type = *typ
		s.Needs = needs
		s.Repos = repos
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

	next, stalled, waiting, err := audit.Next(repo)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if next == nil {
		msg := "nothing is startable — every story has work in flight or landed."
		for _, w := range waiting {
			msg += fmt.Sprintf(" %s waits on %s.", w.ID, strings.Join(w.Waiting, ", "))
		}
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
