---
id: tb-7ad9
title: 'Visual redesign of the web board: kanban columns, stat tiles, status system'
owner: emmanuel
branch: '*/tb-7ad9-*'
paths: [internal/web/**]
---

## Goal

The first web board was flat gray rows — it read as a log, not a board, and
felt dead. Rebuild the page as something a PM recognizes at a glance: specs
as cards in kanban-style status columns, a stat-tile row for the headline
numbers, a proper status color system (icon + label, never color alone),
and visible liveness (pulse + updated time). Still one embedded HTML file,
still read-only by construction.

## Acceptance

- [x] Specs render as cards grouped in status columns (kanban), not rows
- [x] Stat tiles show done / active / stalled / drift counts at a glance
- [x] Status colors follow a validated palette; every status pairs icon + label
- [ ] Light and dark themes both deliberate (tokens, not an automatic flip)
- [ ] Page shows it is live (pulse, last-updated time); still zero write affordances
