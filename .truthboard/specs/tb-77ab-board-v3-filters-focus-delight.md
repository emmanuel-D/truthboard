---
id: tb-77ab
title: 'Board v3: filters, focus, and delight'
owner: emmanuel
branch: '*/tb-77ab-*'
paths: [internal/web/**]
epic: po-experience
priority: 1
---

## Goal

As a PM living on this board daily, I need focus and identity: find a
story instantly, see epics at a glance, keep the done pile from burying
the live work — and feel the board move when reality moves. Minimalist
but detailed; still zero build chain, one embedded file.

## Acceptance

- [x] A filter bar (text search over title/id, epic chips, owner) narrows
      every column instantly, client-side; active filters are obvious and
      clearable in one click
- [ ] The Done column shows only recently-landed stories (the digest
      window) with a "show older" toggle for the rest
- [x] Columns scroll independently so the board itself fits the viewport
- [x] Epic tags carry a stable color dot — fixed assignment from a
      validated categorical palette, never re-shuffled by filtering
- [ ] Status changes animate: cards visibly move between columns on
      refresh (View Transitions, progressive enhancement)
- [ ] A theme toggle (system / light / dark) persists across visits
