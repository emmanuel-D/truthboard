---
id: tb-25dc
title: 'Export the board as a slide deck: delivered stories, by sprint or status, as a PDF'
owner: emmanuel
branch: '*/tb-25dc-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 2
---

## Goal

A PO finishes a sprint and has to tell someone outside the repo what
landed. Today that means screenshotting a kanban column or pasting the
markdown report into a deck by hand. The board already knows exactly what
was delivered, and it derived it from git rather than from anyone's memory
— that is the report nobody wants to write.

Export turns the board into a presentation: a cover, then one slide per
status group, laid out like slides rather than like a web page, saved as
PDF. What goes in it is the reader's choice — which stories (delivered
recently, one sprint, or the whole board), which statuses, and how much
detail each story carries.

The PDF comes from the browser's own print engine — the same engine that
renders the board — so the deck keeps real typography and costs the binary
no new dependency. The page is built for paper: fixed 16:9 slides, no
board chrome, nothing that only makes sense on a screen.

## Acceptance

- [x] An Export button on the board opens a dialog with three scope choices: stories delivered in the digest window, one sprint, or the whole board
- [x] Statuses to include are chosen as chips; delivered scope defaults to done alone
- [x] Detail is chosen as a preset (title only / standard / everything) and every field it controls can be toggled on its own afterwards
- [x] The deck previews on screen before printing, so nobody prints to find out what they got
- [ ] Printing produces 16:9 landscape slides: a cover with the counts and the window it covers, then story slides grouped by status, paginated so no slide overflows
- [x] The deck renders legibly whether or not the print dialog's background graphics are enabled
- [x] Exporting works on a read-only board — it reads, it never writes
- [x] Board filters in force are honoured, and the cover says so
- [x] `go build ./...` and `go test ./...` pass
