---
id: tb-10fb
title: A stale MCP server serves wrong statuses and never says so
owner: emmanuel
branch: '*/tb-10fb-*'
paths:
    - internal/mcp/**
    - internal/lifecycle/**
    - cmd/truthboard/**
    - README.md
epic: agent-loop
priority: 1
type: bug
---

## Goal

As an agent whose only view of the board is the MCP server, I want to be
told when that server is older than the truthboard on this machine, so a
status I am about to trust is one I can trust.

This is not a hypothetical. An MCP server process outlives the release that
started it — the client spawns it once and keeps it — so an upgrade leaves
the agent talking to the old code indefinitely. On 2026-08-14, with v0.17.0
installed, a session's `get_board` reported a story that had been *filed and
never started* as `done`, "work landed on main", and listed it under
`shipped`; `next_spec` answered "nothing is startable". The same repo, read
by the v0.17.0 binary at the same moment, correctly said `planned`, and
`truthboard next` handed the story straight over. The agent's own answer was
the wrong one, and nothing in it looked wrong. Only a cross-check against
the CLI caught it, and only by accident.

Every consequence of that is the exact failure this product exists to
prevent. An unstarted story reported as delivered is a false `done` on the
PO's board. A startable story hidden from `next_spec` is an idle agent told
there is no work — the answer most likely to end a session. And an agent
that re-derives "nothing to do" has no reason to check further.

The precedent is already here and already correct: `truthboard status`
flags a *detached board* older than your binary, because that process keeps
the binary it started with too. The MCP server has the same lifetime and no
such warning. `serverInfo.version` is in the handshake, so the fact is on
the wire — it is just never compared to anything, and no agent thinks to.

The fix belongs on the server, not in the client's discipline: the process
that knows it is stale should say so, in the tool result, where an agent
reading a status cannot miss it. Warn, never refuse — a stale board is
still mostly right, and a server that stopped answering would strand every
session that hit it.

## Acceptance

- [x] **Given** an MCP server whose build is older than the `truthboard` on
  PATH, **when** any status-bearing tool answers (`get_board`, `next_spec`,
  `list_specs`, `get_brief`), **then** the result carries a warning naming
  both versions and the fix — restart the client so the server respawns
- [x] **Given** the server is the same version as the binary on PATH, or
  newer, **then** nothing is added to any result — silence is the default,
  the same courtesy every other truthboard warning extends
- [x] **Given** no `truthboard` on PATH, or one that cannot be run,
  **then** the server answers normally and says nothing: an unanswerable
  comparison is never a warning, and never an error
- [x] **Given** a stale server, **then** it still answers every tool
  normally — a version gap warns, never refuses, because a board that
  stopped answering strands the session that needed it
- [x] The version comparison is a pure function over two version strings,
  unit-tested for: older, equal, newer, a dev/unset build, and an
  unparseable version on either side — no test may shell out to PATH
- [x] **Given** the check itself costs a process spawn, **then** it runs at
  most once per server lifetime, not once per tool call
- [x] `truthboard status` reports running MCP server processes older than
  the installed binary, next to the detached boards it already reports —
  one place that answers "what on this machine is serving old truth?"
- [x] The README's MCP section notes that the server is spawned once and
  outlives an upgrade, and that restarting the client is what picks up a new
  one — the failure is invisible otherwise
