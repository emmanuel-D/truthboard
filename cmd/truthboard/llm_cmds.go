package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/llm"
	"github.com/emmanuel-D/truthboard/internal/report"
)

func runDraft(args []string) int {
	fs := flag.NewFlagSet("draft", flag.ExitOnError)
	owner := fs.String("owner", "", "owner for the drafted specs")
	repo := fs.String("repo", ".", "repository path")
	// Like spec new: the concept is everything before the first flag.
	rest := args
	var conceptParts []string
	for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		conceptParts = append(conceptParts, rest[0])
		rest = rest[1:]
	}
	fs.Parse(rest)
	concept := strings.Join(conceptParts, " ")
	if concept == "" {
		fmt.Fprintln(os.Stderr, `usage: truthboard draft "Brief concept summary" [--owner name] [--repo path]`)
		return 2
	}
	p, err := llm.FromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard draft: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "drafting with %s…\n", p.Name())
	created, err := llm.Draft(p, *repo, concept, *owner)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard draft: %v\n", err)
		return 1
	}
	fmt.Printf("drafted %d stories:\n", len(created))
	for _, s := range created {
		fmt.Printf("  %s  %s\n", s.ID, s.Title)
	}
	fmt.Printf("\nReview and edit the intent (%s), then work them like any story.\n", created[0].File[:strings.LastIndex(created[0].File, "/")])
	return 0
}

// Summary is deliberately not in this file's spirit: it calls no model.
// A PO should not need an API key to learn what shipped, so the summary is
// arithmetic the audit already did, rendered as sentences.
func runSummary(args []string) int {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	withIDs := fs.Bool("ids", false, "include story ids, for a reader who wants to look one up")
	fs.Parse(args)
	sprint := ""
	if fs.NArg() > 0 {
		sprint = fs.Arg(0)
	}
	res, err := audit.Audit(*repo, audit.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard summary: %v\n", err)
		return 1
	}
	s, err := res.Summarise(sprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard summary: %v\n", err)
		return 1
	}
	if err := report.Summary(os.Stdout, s, *withIDs); err != nil {
		fmt.Fprintf(os.Stderr, "truthboard summary: %v\n", err)
		return 1
	}
	return 0
}

func runPlan(args []string) int {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	fs.Parse(args)
	sprint := ""
	if fs.NArg() > 0 {
		sprint = fs.Arg(0)
	}
	p, err := llm.FromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard plan: %v\n", err)
		return 1
	}
	res, err := audit.Audit(*repo, audit.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard plan: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "planning with %s…\n", p.Name())
	text, err := llm.Plan(p, res, sprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard plan: %v\n", err)
		return 1
	}
	fmt.Println(strings.TrimSpace(text))
	return 0
}

func runReview(args []string) int {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	fs.Parse(args)
	sprint := ""
	if fs.NArg() > 0 {
		sprint = fs.Arg(0)
	}
	p, err := llm.FromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard review: %v\n", err)
		return 1
	}
	res, err := audit.Audit(*repo, audit.Options{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard review: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stderr, "reviewing with %s…\n", p.Name())
	text, err := llm.Review(p, res, sprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard review: %v\n", err)
		return 1
	}
	fmt.Println(strings.TrimSpace(text))
	return 0
}
