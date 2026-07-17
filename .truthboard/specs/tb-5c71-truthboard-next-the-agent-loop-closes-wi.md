---
id: tb-5c71
title: 'truthboard next: the agent loop closes with one deterministic call'
owner: emmanuel
branch: '*/tb-5c71-*'
paths:
    - internal/audit/**
    - internal/mcp/**
    - internal/adopt/**
    - cmd/truthboard/**
    - README.md
epic: agent-loop
priority: 1
---

## Goal

"AI, start the next story" currently requires the human (or the agent) to
read the board and pick. That choice should be one deterministic call:
the backlog is already ordered (priority first, unset last, id tie-break),
and planned status already means unclaimed — no branch, no commits.

Add `truthboard next` (CLI) and `next_spec` (MCP): return the first
*planned* spec in backlog order, as the same context packet `brief`
produces, so the answer is immediately actionable. Same repo state, same
answer — two agents asking get the same story, and the second one sees
the board move once the first pushes a branch.

When nothing is planned, say so honestly and point at what exists instead:
stalled stories worth resuming, or `spec new` to write intent. Never
invent work.

The working agreement (AGENTS.md/CLAUDE.md written by adopt) should teach
idle agents the call, so "pick up the next story" needs no human phrasing.

## Acceptance

- [x] `truthboard next` prints the highest-priority planned story as a
      ready-to-work brief; ordering matches the board (priority, then id)
- [ ] specs with any linked work (in-progress, in-review, stalled, done,
      regressed) are never returned — planned only
- [ ] empty backlog: clear message naming stalled count when there is one,
      non-zero exit so scripts can branch on it
- [ ] `next_spec` over MCP returns the same packet; empty backlog returns
      the message, not an error
- [ ] the adopt working agreement mentions next_spec / truthboard next for
      idle agents, and re-running adopt upgrades existing files
- [ ] README and usage text document the command
