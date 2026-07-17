---
id: tb-d135
title: Sprint dates and a derived sprint state
owner: emmanuel
branch: '*/tb-d135-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/web/**
epic: po-experience
priority: 1
---

## Goal

Sprints today are just a slug on specs (tb-463f); nobody can ask "when does
sprint s12 end, and are we on track?" Give sprints their own intent:
a `.truthboard/sprints/<slug>.md` file with `start:` and `end:` dates
(intent, editable like any spec). The sprint *state* stays derived —
future/active/completed comes from today's date vs the window, never typed.
The sprint digest and the web board show the window, days remaining, and
done-vs-total progress for the active sprint.

## Acceptance

- [ ] A sprint file with `start:`/`end:` frontmatter is picked up by audit, digest, and the web board
- [ ] Sprint state (future / active / completed) is derived from the dates — there is no status field to type
- [ ] The active sprint's digest line shows days remaining and stories done vs total
- [ ] A sprint slug referenced by specs but lacking a sprint file still works exactly as today (dates are opt-in)
