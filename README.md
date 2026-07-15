# Truthboard

**Your repo already knows the status. Stop typing it twice.**

Truthboard is a read-only auditor for git repositories: it derives a project
board, a drift report, and a stakeholder digest from repo reality — branches,
merges, patch-equivalence — and never asks a human to update a status.
It doesn't replace your tracker; it tells you where your tracker is lying.

## Usage

```sh
truthboard audit                  # audit the current directory
truthboard audit ~/dev/some-repo  # audit another repo
truthboard audit --format md      # markdown (for a weekly drift issue)
truthboard audit --format json    # machine-readable (for CI/automation)
```

What it reports:

- **Derived board** — every non-integration branch classified as
  `in-progress`, `stalled`, or `done` (merge detected by ancestry *or*
  patch-equivalence, so squash/rebase merges are caught).
- **Drift** — stale promises (work that stopped without landing), shadow work
  (commits that bypassed any branch/MR flow), zombie branches (landed but
  never deleted), and a misconfigured remote default branch if it spots one.
- **Digest** — what actually landed recently, readable by a non-developer.

## Build

```sh
go build ./cmd/truthboard
go test ./...
```

Single static binary, no runtime dependencies beyond `git` itself.

## Status

`0.1.0-dev` — Phase 1 (audit engine + CLI) of [CONCEPT-V2.md](CONCEPT-V2.md).
The inference logic was validated at 100% done-vs-not-done accuracy against
GitHub PR state on real repos before being ported to Go
([CONCEPT-V1.md](CONCEPT-V1.md) §11). Next: GitHub Issues adapter and the
GitHub Action wrapper.
