---
id: tb-ad46
title: Stalled and regressed stories notify someone, not just the board
owner: emmanuel
branch: '*/tb-ad46-*'
paths:
    - internal/audit/**
    - internal/web/**
    - internal/lifecycle/**
epic: po-experience
priority: 1
---

## Goal

The board already derives that work stalled or regressed — but only
tells people who open it. Close the gap: the long-running process we
already have (the detached board / fetch loop) remembers the last
derived status per spec in `.git/truthboard/` and, when a story
*transitions* into stalled or regressed, posts the transition to a
configured webhook URL (generic JSON, Slack-compatible). Still fully
derived: a notification only ever repeats what the audit concluded,
with its evidence line. No transitions, no noise — steady state is
silent, and a story that was already stalled at first sight is baseline,
not news.

## Acceptance

- [ ] A story transitioning to stalled or regressed while a detached board runs posts one notification (id, title, new status, evidence) to the configured webhook URL
- [ ] Steady states and re-derivations produce no repeat notifications; the seen-state survives board restarts
- [ ] Recovery transitions (stalled/regressed back to in-progress or done) notify too — good news is news
- [ ] No webhook configured: behavior is exactly today's, zero calls out
