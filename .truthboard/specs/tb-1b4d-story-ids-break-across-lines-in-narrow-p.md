---
id: tb-1b4d
title: Story ids break across lines in narrow panel chips
owner: emmanuel
branch: '*/tb-1b4d-*'
paths:
    - internal/web/static/app.css
epic: po-experience
priority: 2
type: bug
---

## Goal

At mobile width the planning panel's chips render a story id split over
two lines — `tb-` on one, `0a1b` on the next — and the same happens to
`needs tb-5c6d` in a blocked chip. Seen at 390px on the demo board while
verifying tb-e4c0.

The cause is not a stray rule: `.pid` carries no `overflow-wrap`, and
`overflow-wrap: anywhere` is scoped to `.ptitle` where it belongs. CSS
simply allows a line break **after a hyphen**, and every Truthboard id
contains one. So the id is being treated as two words because it looks
like two words.

An id is a single token — it is copied, searched for, and typed into
`truthboard brief`. Half of one at the end of a line is not something a
reader can use, and it reads as broken layout rather than as wrapping.

Titles must keep breaking anywhere: they are prose, they are long, and
tb-e4c0's whole point was that a story wraps as a unit inside its chip
rather than shattering across it. This is the narrow exception — the
identifier stays whole and the chip grows or the title reflows around it.

Applies to the planning panel (`.pstory .pid`, and the ids inside
`.pblock`) and to the sprint panel's chips, whose `code` element has no
class of its own.

## Acceptance

- [ ] A story id renders as one unbroken token in the planning panel and the sprint panel at 390px
- [ ] The `needs <id>` blocker in a blocked chip keeps its id whole too
- [ ] Story titles still wrap freely inside their chip — the fix is scoped to identifiers, not to the chip
- [ ] No chip overflows its panel and the page still does not scroll sideways at 390px
