---
id: tb-93a3
title: Every story id on the board opens the story
owner: emmanuel
branch: '*/tb-93a3-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 1
type: story
---

## Goal

A story id is the board's one durable handle — it is what the drift
panel names, what the digest lists, what a story's `needs` points at,
what the trailer carries. Everywhere except a card it is dead text.
Reading "Unverified acceptance — tb-3dfb landed with 0/7 criteria
ticked" and wanting to see tb-3dfb means scrolling back to the kanban
and hunting for the card, or typing the id into the filter box. The id
is already the link; it just is not clickable.

Only a card carries `data-spec`, and the click delegation is bound to
`#app`, so nothing inside a dialog can navigate either — a story's
`needs` list names the stories that must land first and cannot reach
them.

Two smaller things on the same surface:

**The type tag breaks across lines.** `.tag` never says
`white-space: nowrap`, so a `× bug` chip at the end of a digest line
splits: the `×` stays and `bug` drops to the next row, still wearing
half a rounded border. It happens wherever a title is long enough to
push the chip to the line end.

**No favicon.** The tab shows the browser's blank-page glyph, which is
what every other tab shows, so a pinned board is unfindable among them.

## Acceptance

- [x] A story id is clickable wherever the board prints one: digest, drift, sprint rows, planning rows, flow, and a card's own chips
- [x] Clicking an id opens that story's detail view, from the landing page and from inside a dialog alike
- [x] Ids inside the detail view navigate: the Needs row reaches the stories that must land first, and an id the board has never heard of stays plain text rather than becoming a link that opens nothing
- [x] An id reads as clickable before it is clicked, and is reachable and operable from the keyboard
- [x] Text that only looks like an id is left alone: branch names, repo names, commit hashes, and the linking instructions keep their plain rendering
- [x] A type tag never splits across two lines
- [x] The board has a favicon that needs no network request, and it works on a light and a dark tab strip
- [x] The deck still prints ids as plain text, since a PDF has nothing to click
