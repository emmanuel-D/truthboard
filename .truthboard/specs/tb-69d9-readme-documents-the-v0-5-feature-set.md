---
id: tb-69d9
title: README documents the v0.5 feature set
owner: emmanuel
branch: '*/tb-69d9-*'
paths:
    - README.md
epic: ship-readiness
priority: 1
type: task
---

## Goal

The README stops at roughly v0.2: nothing on story points, types,
dependencies, sprint dates, the terminal board, the LLM draft/review
commands, webhook-triggered live boards, or stalled/regressed
notifications — and the Status section still says 0.1.0-dev. Bring it up
to the v0.5.0 feature set without bloating the quick start.

## Acceptance

- [ ] Spec-mode section covers points, type, needs (with derived waiting), and dated sprints
- [ ] Terminal board (truthboard board) and LLM assist (draft/review) have sections
- [ ] Web board section documents --notify, the webhook + SSE live mode, and no longer claims a single embedded HTML file
- [ ] Status section reflects the current version and the MCP tool list is complete
