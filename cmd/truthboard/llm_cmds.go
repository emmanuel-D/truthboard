package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/llm"
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
