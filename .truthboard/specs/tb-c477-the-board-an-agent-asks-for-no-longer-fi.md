---
id: tb-c477
title: The board an agent asks for no longer fits in its context
owner: emmanuel
branch: '*/tb-c477-*'
paths:
    - internal/mcp/**
    - internal/audit/**
    - internal/report/**
    - cmd/truthboard/**
    - AGENTS.md
epic: mcp-server
priority: 1
type: bug
---

## Goal

Step 1 of the working agreement — "check the board first: `get_board`" —
fails in this repo. Measured on 2026-08-14 at 97 specs, `get_board`
returned **94,609 characters (~25k tokens)** and was rejected by the MCP
client for exceeding its output cap. The agent that is told to look
before it works cannot look.

Where the payload goes:

| section | chars |
| --- | --- |
| `specs` | 40,687 |
| `drift` | 11,504 |
| `units` | 11,440 |
| `digest` | 4,963 |
| `shipped` | 4,201 |

Two compounding causes. First, `get_board` takes exactly one parameter
(`repo`): there is no way to ask for less than everything, so a done
story from three months ago costs the same tokens as the one in flight.
Second, the payload carries weight nobody asked for — 11,440 of the
11,504 drift characters are `landed_not_deleted`, 48 spent branches that
the text audit itself reports as *clean*, restating detail already in
`units`.

There is also no way to ask a *narrow* question. "Has this already been
filed?" — the single most common thing an agent needs before
`create_spec` — has no cheap answer; the only route is downloading the
whole board and reading it.

This scales the wrong way: every story filed makes the first call of
every agent session more expensive, and this repo is not an unusually
large backlog.

## Acceptance

- [ ] `get_board` accepts narrowing parameters — at least `status`, `epic`, `sprint`, `since` and `limit` — and an unknown or malformed value fails loudly rather than being ignored
- [ ] The unfiltered default stays within a stated token budget on a 100+ spec repo: done stories are summarised (id, title, status, tick counts), not carried in full, and the budget is asserted by a test over a fixture board of that size
- [ ] `drift.landed_not_deleted` stops restating unit detail — branch names or a count with a pointer, not a copy of `units`
- [ ] A `find_spec` call answers "has this been filed?" from a text query, returning matching ids, titles and derived statuses, without the caller fetching the board
- [ ] `truthboard audit --format json` takes the same narrowing flags, so CLI and MCP agree
- [ ] The working agreement written by `init --agents` names the cheap call for step 1, and re-running the command replaces that block rather than duplicating it
- [ ] Nothing here sets, gates or downgrades a status; filtering changes what is *shown*, never what is derived
- [ ] `go test ./...` passes
