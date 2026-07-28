---
id: tb-c469
title: Divers UI/UX improvements
owner: emmanuel
branch: '*/tb-c469-*'
paths:
    - internal/web/static/**
priority: 3
---

## Goal

The board does the right thing and says nothing about it. Every intent
action has feedback of a different shape, or none:

| action | what you see today |
| --- | --- |
| create / edit a story | the dialog closes. That is all |
| retire a story | the dialog closes. That is all |
| tick an acceptance box | nothing on success; an error lands in the hint line |
| assign an owner | a note beside the field |
| push to origin failed | a banner that stays until the next write |

Three mechanisms, no confirmation for the two biggest actions, and a
dialog that vanishes is indistinguishable from a dialog that crashed.

Worse, the board then appears to do nothing for up to three seconds.
Every write sets `last = ""` so the next poll re-renders — but nothing
asks for that poll, so the acting viewer waits out the 3s timer to see
their own change. Other viewers now hear about it immediately over SSE.
The person who made the change is the last to know.

## Acceptance

- [ ] One transient-feedback mechanism, announced to assistive tech,
      dismissable, and honouring prefers-reduced-motion
- [ ] Creating, editing and retiring a story each confirm by name, and
      the retire confirmation says the undo is `git revert`
- [ ] Ticking an acceptance box confirms; failures say so where the
      action happened rather than only in a corner
- [ ] A write refreshes the board immediately instead of waiting out the
      poll interval — the viewer who acted sees it first, not last
- [ ] A push failure stays visible rather than auto-dismissing: "saved
      here but not on origin" is a condition, not an event
- [ ] The board says it is loading on first paint instead of showing
      empty columns
- [ ] Verified in a browser, not only by reading the diff

## Notes

Scope is presentation. Nothing here changes what is derived, what is
written, or when — statuses stay derived from git.
