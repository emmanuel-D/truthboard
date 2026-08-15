---
id: tb-256f
title: The last kanban column is off-screen at every window width
owner: emmanuel
branch: '*/tb-256f-*'
epic: po-experience
priority: 1
type: bug
points: 2
---

## Goal

The kanban lays out as fixed 17.5rem columns (`app.css:163`), so five
statuses need `5 × 280 + 4 × 11.2 = 1444.8px`. The page is capped at 88rem
with 2.2rem of padding a side, which leaves the board `1337.6px` — and that
cap does not move, so the arithmetic is the same on a 1440px laptop and on a
32-inch display. The last column is always cut off, and the fix a wider
window would normally be is not available.

Six columns is worse and is the ordinary case for anyone using a forge:
`in-review` makes it `1736px` against the same `1337.6px`.

The row does scroll sideways, so nothing is unreachable. That is not the
same as being seen: DONE is the column that answers "what did we ship", and
a board whose answer is permanently half past the right edge is a board
people learn to distrust. On a wide screen there is also plenty of empty
space beside it, which makes the clipping look like a rendering fault
rather than a scroll affordance.

Columns should share the width the page has, keeping a floor below which
they scroll instead of shrinking — the phone behaviour, which is correct
and must stay.

## Acceptance

- [ ] Five columns fit without horizontal scrolling on a desktop window
- [ ] Six columns (with in-review) fit too, or come closer without breaking the floor
- [ ] Below the floor the row still scrolls rather than squeezing cards unreadably
- [ ] Phone and tablet layouts are unchanged
- [ ] `go test ./...` passes
