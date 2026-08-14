---
id: tb-a066
title: The agent that lands the work also ticks the acceptance it satisfied
owner: emmanuel
branch: '*/tb-a066-*'
paths:
    - internal/audit/**
    - internal/spec/**
    - internal/mcp/**
    - internal/adopt/**
    - internal/report/**
    - internal/tui/**
    - internal/web/**
    - cmd/truthboard/**
epic: agent-loop
priority: 1
type: bug
---

## Goal

Stories keep deriving done with their acceptance checklist untouched —
tb-dcd6 landed 0/1, tb-8007 0/3, tb-5c71 1/6. The status is right (git
proved the work landed) but the criteria never got read back, so nobody
can tell which promises were actually checked and which were assumed.

Three things cause it, and all three need fixing:

1. **Ticking is expensive.** The only write path is `update_spec` with a
   *full replacement body* — an agent must re-emit the whole markdown to
   flip one `[ ]`. So it doesn't.
2. **Nothing asks for it.** The brief, `next_spec` and the working
   agreement `adopt` writes all stop at "satisfy the acceptance criteria";
   none of them says to record that you did.
3. **Forgetting is silent.** Every other kind of drift is reported. An
   unverified promise is not, so it costs nothing to skip.

Ticking stays *intent* — a human-or-agent claim, exactly like the hold
note. Git still derives the status alone: an unticked criterion must
never block, delay or downgrade a derived `done`. It just stops being
invisible.

## Acceptance

- [x] `check_acceptance` (MCP) and `truthboard check <id>` tick criteria by index or unique substring, editing only those checkbox lines — no body rewrite
- [x] Both accept several criteria at once, an `all` form, and an uncheck form; an ambiguous, unknown or out-of-range selector fails loudly and prints the numbered checklist it saw
- [x] `get_brief` and `next_spec` render the acceptance checklist with per-criterion indices and tick state, and close with the explicit step: tick what you satisfied, commit it with the trailer
- [x] `next_spec` warns, before handing out new work, when the story the agent most recently landed still carries unticked criteria — naming it and the count
- [x] Unverified acceptance is drift: a spec derived done whose criteria are not all ticked appears in `truthboard audit`, the markdown report, the TUI, `get_board` JSON and the web board
- [x] A done spec with no checklist at all is not drift — there is nothing to verify, and old specs must not turn the board red
- [x] The working agreement `adopt` writes carries the tick step in its marker block, and re-running `init --agents` on an already-wired repo replaces that block rather than duplicating it
- [x] Statuses stay derived end to end: no code path here sets, gates or downgrades a status, and a done spec with 0/N ticks still reads done
- [x] `go test ./...` passes
