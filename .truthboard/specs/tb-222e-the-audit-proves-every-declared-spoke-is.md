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

- [ ] The audit reports a drift finding per declared spoke that has a local
      checkout but is not wired: missing MCP registration, an MCP argument that
      no longer resolves to the hub, or a missing working agreement.
- [ ] The finding names the spoke and the exact command that fixes it.
- [ ] A spoke with no local copy is not a wiring finding — that is already the
      existing unreadable-spoke report, and saying it twice trains people to
      ignore both.
- [ ] The commit-msg nudge is reported when absent but never as a hard finding:
      it is optional (`--hooks`), and a warning for an opt-in feature cries wolf.
- [ ] A single-repo workspace produces no such finding, ever.
- [ ] The audit stays read-only: it detects, it never wires.
- [ ] `docs/multi-repo.md` documents the finding alongside the unreadable-spoke
      one.
