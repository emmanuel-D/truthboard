---
id: tb-7784
title: The markdown report drops the sprint points and dates the terminal shows
owner: emmanuel
branch: '*/tb-7784-*'
paths:
    - internal/report/**
epic: po-experience
priority: 2
type: bug
---

## Goal

`report.go` has two renderings of the sprint rollup and they disagree.
`Terminal` calls `sprintPoints` and `sprintWindow`, so a developer at a
prompt sees:

    s12  2/4 done · 13/21 pts · 2026-07-27 → 2026-08-07 · active, 4d left

`Markdown` writes only `%d/%d done` plus the open list, so the same audit
through `--format md` says:

    - **s12** — 2/4 done · open: `tb-3e4f` …, `tb-4a5b` …

Points, the date window, the derived state and the days remaining are all
silently absent. Markdown is the format that gets pasted into a status
doc, a wiki page or an email — the one most likely to reach someone who
is not a developer — so the renderer that drops the business-legible
numbers is precisely the wrong one to drop them.

Found while answering "can a PO see what we achieved and for how many
points": the terminal could answer it and the paste-able format could not.

## Acceptance

- [ ] The markdown sprint line carries points done vs total, the unestimated count, the date window, the derived state, and days remaining — the same facts the terminal renders
- [ ] Undated sprints and unestimated sprints degrade in markdown exactly as they do in the terminal, with no empty separators left behind
- [ ] The shared rendering is factored so a third surface cannot drift from these two again
- [ ] A test asserts the two renderers agree on the same rollup, so this cannot silently regress
