package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/emmanuel-D/truthboard/internal/adopt"
	"github.com/emmanuel-D/truthboard/internal/lifecycle"
)

// runUninstall takes the wiring back out of a repository. It plans by
// default and writes only with --apply — the same contract as `mirror`,
// and for the same reason: the surgery runs through files the adopter owns,
// so they get to read it before it happens.
func runUninstall(args []string) int {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	apply := fs.Bool("apply", false, "actually remove the wiring (without this, the plan is printed and nothing is written)")
	specs := fs.Bool("specs", false, "also delete .truthboard/ — every story you have written. Kept by default")
	fs.Parse(args)

	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}

	// The board holds the pid recorded in the run state about to be cleared,
	// so it stops first — and only on a real run, since a plan must leave the
	// repository exactly as it found it.
	if *apply {
		if state, err := lifecycle.Load(repo); err == nil && state != nil {
			msg, err := lifecycle.Stop(repo)
			if err != nil {
				fmt.Fprintf(os.Stderr, "truthboard: could not stop the board: %v\n", err)
				return 1
			}
			fmt.Printf("  board: %s\n", msg)
		}
	}

	log, err := adopt.Uninstall(repo, *apply, *specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "truthboard: %v\n", err)
		return 1
	}
	if *apply {
		fmt.Printf("removed truthboard's wiring from %s\n", repo)
	} else {
		fmt.Printf("plan for %s — nothing has been written:\n", repo)
	}
	for _, line := range log {
		fmt.Printf("  %s\n", line)
	}

	if !*apply {
		fmt.Println()
		fmt.Println("Run it for real with --apply.")
		return 0
	}
	exe, exeErr := os.Executable()
	if exeErr != nil {
		exe = ""
	} else {
		exe = filepath.Clean(exe)
	}
	for _, line := range adopt.Binary(exe) {
		fmt.Println(line)
	}
	return 0
}
