---
id: tb-222e
title: The audit proves every declared spoke is still wired, or says which is not
branch: '*/tb-222e-*'
paths:
    - internal/audit/**
    - docs/multi-repo.md
epic: agent-loop
priority: 2
needs:
    - tb-fbb0
---

## Goal

Wiring a spoke once is not the same as a spoke staying wired. A fresh clone of
a spoke, a hand-edited `.mcp.json`, a spoke declared after the last setup run —
each leaves a repo that the board watches for proof but whose agents have no
board, no agreement, and no nudge. Nothing reports that today, so the failure
is silent until trailerless commits show up as shadow work.

Make "every declared repo is bound" a derived fact like every other status:
a drift finding, computed from what is on disk, naming the spoke and the fix.

## Acceptance

- [x] The audit reports a drift finding per declared spoke that has a local
      checkout but is not wired: missing MCP registration, an MCP argument that
      no longer resolves to the hub, or a missing working agreement.
- [x] The finding names the spoke and the exact command that fixes it — and
      that command works as printed, including on a hub with nothing new to
      declare.
- [x] A spoke with no local copy is not a wiring finding — that is the existing
      unreadable-spoke report, and saying it twice trains people to ignore both.
- [x] The commit-msg nudge is never a finding on its own: it is opt-in
      (`--hooks`), so it is named only when a spoke is already being reported
      for a real gap *and* the hub has a nudge the spoke lacks — a demonstrated
      intent mismatch rather than a warning nobody asked for.
- [x] A single-repo workspace produces no such finding, ever.
- [x] The audit stays read-only: it detects, it never wires.
- [x] The finding renders everywhere the board does — terminal, markdown, JSON,
      TUI, web — and counts toward the web drift tile.
- [x] `docs/multi-repo.md` documents the finding alongside the unreadable-spoke
      one.
