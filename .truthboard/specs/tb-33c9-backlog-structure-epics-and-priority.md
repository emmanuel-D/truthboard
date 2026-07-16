---
id: tb-33c9
title: 'Backlog structure: epics and priority'
owner: emmanuel
branch: '*/tb-33c9-*'
paths: [internal/spec/**, internal/audit/**, internal/report/**, internal/web/**]
epic: po-experience
priority: 1
---

## Goal

As a PO starting a project from scratch, the specs folder must work as a
real backlog: stories grouped under epics, ordered by priority — so
"what should we do next?" has an answer on the board, not in someone's
head. Epic and priority are *intent* (human/agent editable); they never
affect derived statuses.

## Acceptance

- [x] Frontmatter gains `epic:` (free-form slug) and `priority:` (1=now,
  2=next, 3=later; unset sorts last); both optional, both round-trip
  through Save
- [x] **Given** planned specs with priorities, **then** every surface (CLI
  board, markdown report, web UI, JSON) orders within a status by
  priority, then id
- [x] Cards and rows show the epic as a small tag; specs without an epic
  render unchanged
- [x] `list_specs` output includes epic and priority so agents can pick
  "highest-priority planned story" without heuristics
