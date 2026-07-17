---
id: tb-cc9f
title: Assign in one click, and an unassigned filter
owner: emmanuel
branch: '*/tb-cc9f-*'
paths:
    - internal/web/**
epic: po-experience
priority: 2
---

## Goal

Owner filter chips exist (tb-77ab) and the editor can set an owner, but
the daily gesture — "give this story to X" — takes opening the full
editor, and stories nobody owns can't be filtered to at all.

Two additions, both intent edits like any other (plain git diffs):

- **Quick assign** in the card detail view: an owner control that offers
  the owners already on the board plus free text, saving on change via
  the existing PUT /api/specs route. Hidden on read-only (shared) boards.
- **Unassigned chip** in the owner filter row so a standup can start from
  "what has no owner".

## Acceptance

- [ ] the detail view assigns an owner in one interaction (existing
      owners suggested, new names typable) and the card avatar updates
      on the next poll
- [ ] an "unassigned" chip filters to stories with no owner, and
      combines with the other filters
- [ ] on a read-only board the assign control is hidden along with the
      other editing affordances
