---
id: tb-1f09
title: An acceptance tick can name its evidence, so it can be re-derived
owner: emmanuel
branch: '*/tb-1f09-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/mcp/**
    - internal/report/**
    - internal/web/**
    - cmd/truthboard/**
epic: agent-loop
priority: 2
type: story
needs:
    - tb-a066
---

## Goal

`tb-a066` made ticking cheap and made a missing tick visible, which was
the right first move. What it could not fix is what a tick *is*: an
agent's word that a promise came true, recorded once and never checked
again. The board now knows a criterion was ticked. It still has no way
to know whether the thing it claims is still true.

That is the same position statuses were in before this tool existed —
someone asserts, everyone trusts, the assertion rots quietly. Status
escaped it by pointing at evidence git can re-read on every audit. A
tick should be able to do the same: let a criterion name what backs it —
a test, a CI check, a path that must exist — so the claim is re-derived
rather than remembered, and goes red on its own when the evidence
disappears.

Deliberate limits. Evidence stays **optional**: `tb-a066`'s bar is that
an unticked criterion is visible, not that every criterion is machine-
checkable, and prose criteria ("a PO can read this") must stay tickable
by hand. And this stays intent, like the hold note: a criterion whose
evidence vanished is *drift*, never a status change — git alone still
derives done.

## Acceptance

- [ ] A criterion may carry evidence — a test name, a CI check name, or a path — written in the markdown and readable by hand, without breaking specs that carry none
- [ ] `check_acceptance` (MCP) and `truthboard check` record evidence when given it, editing only the lines they touch, and still accept a bare tick
- [ ] Every audit re-derives evidence-backed ticks: a named test or path that no longer exists is reported as drift, naming the story, the criterion and what went missing
- [ ] Evidence that cannot be verified in the current checkout is reported as unverifiable, distinctly from evidence that was checked and failed
- [ ] The distinction between an unticked criterion, a ticked-by-hand criterion and a ticked-with-evidence criterion is visible in `get_brief`, `truthboard audit`, `get_board` JSON, the TUI and the web board
- [ ] Nothing here sets, gates or downgrades a status: a done story with broken evidence still reads done, and appears in drift
- [ ] `go test ./...` passes
