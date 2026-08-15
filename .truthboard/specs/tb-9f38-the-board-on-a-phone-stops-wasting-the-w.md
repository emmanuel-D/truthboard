---
id: tb-9f38
title: The board on a phone stops wasting the width it has
owner: emmanuel
branch: '*/tb-9f38-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 1
type: bug
---

## Goal

Checked at a real 390px viewport, not a resized desktop window — Chrome
will not size a window below ~600px, which is why these survived. Four
places give away width a phone does not have, and one of them is the
first thing on the page.

**Stat tiles.** `repeat(auto-fit, minmax(8.5rem, 11rem))` counts columns
against the 11rem maximum, so 22.4rem of usable width fits one track,
not two: four tiles stack in four rows, each 176px wide in a 390px
screen, with the right half of every row empty. Two 8.5rem tracks would
have fitted all along.

**The summary list.** `.sumlist` never zeroes the UA's
`padding-inline-start`, so every delivered story is pushed 40px right —
12% of the screen, on the longest list on the page. The margin reset
covers margins only. This one is wrong at every width, not only on a
phone, so it is corrected at every width; the other three are phone-only
and belong behind the breakpoint.

**The editor's field grid.** `1fr 1fr 7rem` at 390px computes to
91px | 91px | 112px: the fixed track meant for the *narrow* field ends
up the widest of the three, and Owner and Epic clip their own values.

**The flow metrics.** `.fmname` has `min-width: 14rem` — 224px of a
322px panel — so the figure beside it wraps or does not, depending on
how long it is. "under a minute" takes its own line and "5m" does not,
and the two rows stop reading as the same kind of row.

## Acceptance

- [x] Stat tiles sit two to a row at 390px and fill the width between them
- [x] Delivered stories start at the panel's left edge, not 40px inside it
- [x] The editor's short fields share the width evenly at 390px, and no field clips its own value
- [x] Both flow metric rows lay out the same way at 390px regardless of how long the figure is
- [x] Above the phone breakpoint the three phone-only fixes change nothing: desktop tiles, editor grid and flow rows keep their current layout
- [x] The summary indent is gone on a desktop too, since the gutter was never right at any width
- [x] The page still has no horizontal overflow at 390px, and both dialogs still measure exactly 94vw
