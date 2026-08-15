---
id: tb-f019
title: A panel lays itself out by its own width, not the window's
owner: emmanuel
branch: '*/tb-f019-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 1
type: bug
---

## Goal

Measured at real tablet viewports — 768, 820, 1024, 1180 — the way the
phone was. The band mostly holds, and where it does not, one cause runs
underneath: layout decisions are keyed to the *window* when what they
actually depend on is how wide the element ended up.

**Branch rows shred inside a half-width panel.** The worst of it. The
stacked branch layout is behind `@media (max-width: 40rem)`, so at a
768px viewport — well above that — the desktop five-column flex row
applies inside a panel that is only 352px wide, because the panel is one
half of a `.grid2` pair. Branch names and their evidence then compete
for the same short line and break a word at a time: rows measure 102px
each instead of 30px, and the panel carries fifty of them. Measured by
panel width rather than window width, the row degrades steadily —
660px: 30px rows · 546px: 47px · 472px: 63px · 414px: 79px · 352px:
102px. The window was never the thing that mattered.

**The panel pair un-pairs at 768px, by three pixels.** `.grid2` asks for
`minmax(21rem, 1fr)`; two of those plus the gap need 689.6px and an iPad
portrait offers 686.3. Drift and claims, branches and digest, all stack
into one column on the most common tablet there is.

**Stat tiles wrap 3 + 1.** `auto-fit` counts tracks against the 11rem
maximum again, so three fit and the fourth drops to a row of its own
with ~190px of dead space left beside it.

**The editor split misses iPad landscape by 64px.** The two-pane editor
starts at 68rem (1088px). At 1024 the dialog is already at its full
58rem — 928px — and shows one pane at a time with half of itself empty,
which is exactly the thing the split was written to stop.

## Acceptance

- [x] Branch rows stack whenever their panel is narrow, at any window width, and the rule reads off the panel rather than the window
- [x] A branch row inside a 352px panel is shorter and unbroken compared with the shredded five-column row it replaces
- [x] Branch rows keep the desktop five-column layout in a full-width panel
- [x] The phone still gets the stacked branch layout it had, now from the same rule rather than a second one
- [x] Paired panels sit side by side at a 768px viewport
- [x] Stat tiles fill their row at a 768px viewport instead of leaving a gap and a lonely tile
- [x] The editor splits into two panes at 1024px, and each pane is no narrower than the ones already shipped at 1200px
- [x] Below the split width the tabs still come back, and the JS breakpoint still agrees with the stylesheet
- [x] No horizontal overflow at 768, 1024 or 1180, and phone and desktop layouts are unchanged
