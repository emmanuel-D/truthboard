---
id: tb-e4c0
title: The board shows the sprint about to start, not just the one running
owner: emmanuel
branch: '*/tb-e4c0-*'
paths:
    - internal/web/**
epic: po-experience
priority: 1
needs:
    - tb-0274
---

## Goal

tb-0274 put the planning rollup on the audit `Result`, so `/api/board`
already serves it — every viewer of a running board is receiving the
sprint-start summary and seeing none of it. The sprint panel shows the
iteration that is *running*; nothing shows the one about to start, which
is the meeting people actually gather for.

Render it. Next to the existing sprint panel, a planning panel for the
target sprint: its window and days, what rolls over from the closing
sprint with each story's derived status, what is already committed, the
ready and blocked candidates in backlog order with blockers named, and
committed points against what the last sprint landed.

Two things the panel must not do. It must not imply a velocity — the
reference is one prior sprint and reads as such, matching the facts block
in `internal/llm/plan.go`. And it must stay honest about rollover that is
*stalled* rather than in progress: a stalled rollover story is the most
useful thing on the panel and must not be flattened into "open".

The panel is read-only. Moving a story into the target sprint is already
possible through the existing intent editor, and planning is a
conversation — the board reports it, it does not run it.

Terminal (`audit`) and markdown (`--format md`) rendering stay out
deliberately: `truthboard plan` narrates for humans and `--format json`
carries the data, so a second text rendering earns nothing yet.

The existing sprint panel gets fixed in the same pass, because the new
panel would otherwise inherit its defect. Each open story there is emitted
as three flat siblings — icon, id, title — directly inside the wrapping
flex row (`sprintsPanel`, `internal/web/static/app.js`), so at narrow
widths they wrap independently and a story's id can end up on a different
line from its title, beside the next story's icon. Nothing in the
`max-width: 40rem` block addresses `.sprow` at all. Stories must wrap as
units, in both panels.

## Acceptance

- [x] The web board renders a planning panel from the `plan` object already on `/api/board` — no new endpoint, no second audit
- [x] The panel shows the target sprint's window and length, or says plainly that no sprint is waiting to start
- [x] Rollover stories show their derived status, with stalled visually distinct from in-progress
- [x] Ready and blocked candidates appear in backlog order; blocked ones name the `needs:` ids blocking them
- [x] Committed points appear against the reference sprint's landed points, labelled as one prior sprint rather than a velocity or an average
- [x] The panel is absent, not empty, when the audit produced no plan
- [x] The live-update path refreshes the panel like every other section, with no full reload
- [ ] The panel is readable at mobile width, like the rest of the board
- [x] In both the planning panel and the existing sprint panel, each story's icon, id and title wrap as one unit rather than as independent flex children, and stories are separated by something other than a gap
