package adopt

import (
	"fmt"
	"path/filepath"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
	"github.com/emmanuel-D/truthboard/internal/workspace"
)

// Spokes wires every declared spoke that has a local checkout, so a
// multi-repo workspace costs exactly what a single repo costs to adopt.
// Declaring `path:` is the adopter saying "that checkout is mine and it is
// there" — enough to write into it, as long as every write is reported.
//
// A spoke with no local copy is skipped by name: adoption never clones, and
// it never touches the board server's mirror of a spoke (nobody opens an
// agent in a mirror). One unusable spoke is reported and stepped over — the
// others still get wired, because a workspace half-adopted silently is the
// failure this whole feature exists to remove.
func Spokes(hub string, hooks bool) ([]string, error) {
	ws, err := workspace.Load(hub)
	if err != nil || ws == nil {
		// A malformed manifest is the audit's finding to report, not a
		// reason to fail adoption of the hub that just succeeded.
		return nil, nil
	}

	var log []string
	step := func(format string, a ...any) { log = append(log, fmt.Sprintf(format, a...)) }

	for _, r := range ws.Repos {
		checkout, err := ws.Checkout(r)
		if err != nil {
			step("spoke %s: not wired — %v", r.Name, err)
			step("  once it is checked out, re-run this command to wire it")
			continue
		}
		if _, err := gitrepo.Run(checkout, "rev-parse", "--is-inside-work-tree"); err != nil {
			step("spoke %s: not wired — %s is not a git repository", r.Name, r.Path)
			continue
		}
		hubArg, err := hubRelativeTo(checkout, hub)
		if err != nil {
			step("spoke %s: not wired — %v", r.Name, err)
			continue
		}

		step("spoke %s (%s) — MCP server points at the hub: %s", r.Name, r.Path, hubArg)
		lines, err := wire(checkout, hooks, hubArg, spokeAgreementText(ws, hubArg), ws)
		if err != nil {
			step("  failed: %v", err)
			continue
		}
		for _, line := range lines {
			step("  %s", line)
		}
	}
	return log, nil
}

// hubRelativeTo renders the hub as a path relative to the spoke, in slash
// form: the MCP config carrying it is committed and shared, so this machine's
// absolute paths must never reach it. A relative hub also states the layout
// the workspace assumes — side-by-side checkouts — where an absolute one
// would silently work here and break on every other machine.
func hubRelativeTo(spoke, hub string) (string, error) {
	spokeAbs, err := filepath.Abs(spoke)
	if err != nil {
		return "", err
	}
	hubAbs, err := filepath.Abs(hub)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(spokeAbs, hubAbs)
	if err != nil {
		// Different volumes on Windows: no relative path exists, and an
		// absolute one in a shared file would be worse than none.
		return "", fmt.Errorf("no relative path from the checkout to the hub — wire it by hand")
	}
	if rel == "." {
		return "", fmt.Errorf("declared path points at the hub itself")
	}
	return filepath.ToSlash(rel), nil
}
