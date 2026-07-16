---
id: tb-22b8
title: 'One-command agent adoption: truthboard init --agents'
owner: emmanuel
branch: '*/tb-22b8-*'
paths: [cmd/truthboard/**, internal/adopt/**]
epic: agent-loop
priority: 1
---

## Goal

As a tech lead, I want AI tools like Claude Code to *always* route their
work through Truthboard — get the task via `get_brief`, keep the board
honest via `Spec:` trailers — without me re-explaining the working
agreement in every session. One command should wire a repo for agents:
the MCP registration, the standing instructions, and a gentle nudge when
a commit forgets its trailer.

## Acceptance

- [x] **Given** a fresh repo, **when** I run `truthboard init --agents`, **then**
  it writes `.mcp.json` (truthboard MCP server) and an `AGENTS.md` working
  agreement (brief before work, trailer in every commit, board before new
  work), and appends a pointer to `CLAUDE.md` if one exists
- [x] **Given** `--hooks` is passed, **then** a `commit-msg` hook is installed
  that **warns** (never blocks) when a commit message has no `Spec:` trailer
  and no obvious exempt prefix (merge/revert/fixup)
- [x] Running it twice is idempotent — no duplicated blocks, no clobbered files
- [x] Plain `truthboard init` behavior is unchanged
