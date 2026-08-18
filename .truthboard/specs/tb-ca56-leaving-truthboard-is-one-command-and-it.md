---
id: tb-ca56
title: Leaving truthboard is one command, and it leaves nothing behind
owner: emmanuel
branch: '*/tb-ca56-*'
paths:
    - internal/adopt/**
    - cmd/truthboard/**
    - README.md
    - docs/**
epic: ship-readiness
priority: 2
type: story
---

## Goal

`truthboard init --agents --hooks` writes into six places. Four of them sit in
the working tree and a reader can find them. Two do not: the nudge inside
`.git/hooks/commit-msg`, and the run state in `.git/truthboard/`. Neither ever
appears in `git status`.

So the pilot who decides against truthboard deletes the files they can see,
removes the binary, and then every commit they make prints
`truthboard: no 'Spec: <id>' trailer…` — a warning about a tool that is no
longer installed, from a file they cannot see. That is the last thing the
product says to someone who was evaluating it.

The three that are merged into files the user owns are the ones hand-removal
gets wrong: a `commit-msg` hook that adoption deliberately inserted into
(their own script still has to survive), a `.mcp.json` carrying other servers
(a dangling truthboard entry whose binary is gone fails every agent's MCP
connection silently at session start), and `package.json` scripts. Adoption
already knows how to do this surgery precisely — marker blocks, and
`legacyNudges` for exact-match proof that we authored a hook. Uninstall is
that machinery run backwards.

Two things stay out of scope on purpose. Stories in `.truthboard/` are the
user's intent and their history — never deleted without being asked. And git
history is left alone: past `Spec:` trailers are inert commit text.

Framed for adoption, not departure: "how do I get out" is a question pilots
ask before they say yes, and today the honest answer is "hand-edit four files,
one of them hidden inside `.git/`."

Scope: internal/adopt/**, cmd/truthboard/**, README.md, docs/**.

## Acceptance

- [x] `truthboard uninstall [repo]` prints the plan and writes nothing without `--apply` — the same dry-run-by-default contract as `mirror`
- [x] Content that was in AGENTS.md and CLAUDE.md before adoption is byte-identical after uninstall; only the marker block leaves, and a file that was nothing but the block is removed
- [x] A `commit-msg` hook that was someone else's keeps its own script — only the nudge leaves, whether it is the current one or any version in `legacyNudges`; a hook truthboard wrote alone is deleted; a nudge we cannot prove we authored is reported and left untouched
- [x] `.mcp.json` and `.vscode/mcp.json` lose only the truthboard server, other servers survive, and a file left with no servers at all is removed
- [x] npm scripts still verbatim ours are removed; ones the user has since changed are kept and reported as kept
- [x] The detached board is stopped and `.git/truthboard/` is cleared, so nothing keeps running and no state survives
- [x] `.truthboard/` is never touched without an explicit `--specs`, and the output says where the stories still are
- [x] The output closes by naming how to remove the binary itself, since that is the half install.sh owns
- [x] The README documents the exit path where a reader deciding whether to adopt will meet it
- [x] `go test ./...` passes
