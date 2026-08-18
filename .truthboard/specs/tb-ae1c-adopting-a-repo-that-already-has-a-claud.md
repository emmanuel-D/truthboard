---
id: tb-ae1c
title: Adopting a repo that already has a CLAUDE.md says so before you run it
owner: emmanuel
branch: '*/tb-ae1c-*'
paths:
    - README.md
    - docs/**
epic: ship-readiness
priority: 2
type: task
---

## Goal

`init --agents` behaves correctly against a repo that already has its own
`CLAUDE.md`, `AGENTS.md`, `commit-msg` hook, `.mcp.json` or `package.json`
scripts: everything it writes is marker-delimited and idempotent, existing
content survives, the hook nudge is exit-code-neutral so someone else's hook
still decides the outcome, and existing npm scripts of the same name are
kept. Verified by running it — the wiring is not the problem.

The problem is that no reader can know this before they run it. Nothing in
the README or `docs/**` says what adoption does to a file you already own,
and the one file most likely to be there is the one people have tuned by
hand over months. "Will this eat my CLAUDE.md?" is asked *before* the
command, and an unanswered version of that question is a repo that never
gets adopted — the tool never gets to prove it was safe.

This is not a behaviour change. It is stating a guarantee the code already
keeps, at the moment the reader needs it.

Scope: README.md, docs/**.

## Acceptance

- [x] The README states that an existing AGENTS.md and CLAUDE.md are appended to and never rewritten, and names the marker block as what a re-run replaces
- [x] It says the same for a `commit-msg` hook that already exists — the nudge is inserted warn-only and never changes what that hook decides
- [x] It says the same for a `.mcp.json` that already carries other servers, and for `package.json` scripts the reader already has
- [x] The guarantee is reachable from the README's map, so a reader deciding whether to adopt finds it without reading the adoption section end to end
- [ ] `go test ./...` passes
