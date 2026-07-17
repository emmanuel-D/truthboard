---
id: tb-c56b
title: 'Spec dependencies: needs as intent, readiness derived'
owner: emmanuel
branch: '*/tb-c56b-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/mcp/**
    - internal/web/**
epic: agent-loop
priority: 1
---

## Goal

"tb-A must land before tb-B starts" has no home today, so `truthboard
next` can hand an agent a story whose foundation doesn't exist yet.
Add `needs: [tb-xxxx, ...]` frontmatter — pure intent, editable
everywhere intent is. Everything else is derived: a planned story with
unmet needs is *waiting*, `next`/`next_spec` skips it (returning the
highest-priority story that is actually startable), and the board shows
what it waits on. Waiting is not a new typed status — it is arithmetic
over the same derived statuses, like sprints.

## Acceptance

- [ ] `needs:` accepted in frontmatter, `spec new --needs`, MCP create/update, and the web editor; unknown ids are rejected with the known-ids list
- [ ] `truthboard next` / MCP `next_spec` never returns a story with an un-done need; when only waiting stories remain it says what they wait on
- [ ] Board and TUI cards show waiting state with the blocking ids; the detail view lists each need with its live derived status
- [ ] A dependency cycle is reported as a loud finding naming the cycle, never silently ignored
- [ ] Specs without `needs:` behave exactly as today
