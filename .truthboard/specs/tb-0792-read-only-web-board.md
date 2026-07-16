---
id: tb-0792
title: Read-only web board
owner: emmanuel
branch: '*/tb-0792-*'
---

## Goal

`truthboard ui` serves the spec board + drift report as a local web page for
the PM/PO audience (the interview's "my PM would riot" objection). Strictly
read-only: no buttons that mutate anything, because there is nothing to
mutate — the page just renders the derived Result.

## Acceptance

- [x] `truthboard ui` opens a localhost page rendering board, drift, and digest
- [ ] Zero write endpoints; the page carries a "derived from git, never typed" banner
- [x] Static assets embedded via go:embed — still a single binary
- [x] Auto-refreshes when the repo changes (poll or fs-watch)
