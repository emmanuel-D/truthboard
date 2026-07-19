---
id: tb-dcef
title: 'Workspace board UX: repo filter chips and per-repo drill-down'
owner: emmanuel
branch: '*/tb-dcef-*'
paths:
    - internal/web/static/**
    - internal/tui/**
epic: multi-repo
priority: 2
---

## Goal

A workspace board mixes N repos into one truth — good for the PO, noisy for an engineer who lives in one spoke. Give the board a repo dimension the way epics already have one: filter chips per workspace repo (hub included) that narrow branches, digest, drift, and story cards (a story matches a repo when a linked branch or a `per_repo` entry names it); the meta line's workspace list becomes clickable; the TUI gets a repo cycle key alongside the existing e/s/a filters. Follow the existing filter-chip pattern (owners, epics) — no new UI paradigm.

## Acceptance

- [ ] Web board: one filter chip per workspace repo; selecting it narrows cards, branches, digest, and drift to that repo; chips only appear when a workspace exists.
- [ ] A `repos:` story appears under every repo it declares; a plain story appears under repos its linked branches live in.
- [ ] TUI: a key cycles the repo filter, shown in the header like the other filters.
- [ ] Zero visual change for single-repo boards.
