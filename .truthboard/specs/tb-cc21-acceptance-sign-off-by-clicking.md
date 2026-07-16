---
id: tb-cc21
title: Acceptance sign-off by clicking the checkbox
owner: emmanuel
branch: '*/tb-cc21-*'
paths: [internal/web/**]
epic: po-experience
priority: 1
---

## Goal

As a PO, signing off an acceptance criterion should be one click on the
checkbox I'm already reading — not markdown surgery in a textarea. The
rendered checkboxes in the story detail become live controls: a click
flips `[ ]`/`[x]` in the spec body and saves, same as any intent edit.
Statuses remain untouched — sign-off records *verification*, the board
still derives *delivery* from git.

## Acceptance

- [x] In the detail view, clicking an acceptance checkbox toggles it and
      persists to the spec file (visible in git diff and the dirty nudge)
- [x] The card's n/m progress bar updates on the next refresh
- [ ] A failed save reverts the checkbox and surfaces the error
- [x] Checkboxes in the editor's Preview tab stay inert (it's a preview)
- [x] A short hint tells the PO the boxes are clickable sign-off
