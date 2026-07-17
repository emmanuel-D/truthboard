---
id: tb-8dd7
title: 'Terminal board: a read-only Bubbletea TUI'
owner: emmanuel
branch: '*/tb-8dd7-*'
paths:
    - internal/tui/**
    - cmd/**
epic: dev-experience
priority: 2
---

## Goal

The original blueprint's Phase 6 — an interactive terminal dashboard for
mouse-free developers — never landed. `truthboard board` opens a Bubbletea
TUI rendering the same derived Result the web board shows: kanban columns,
drift report, digest. Read-only for status (nothing to mutate — statuses
are derived), with keyboard navigation, a story detail pane, and the same
filters the web board has (epic, sprint, type, assignee). Refreshes when
the repo changes.

## Acceptance

- [ ] `truthboard board` renders columns, drift, and digest in the terminal via Bubbletea
- [ ] Arrow/vim keys navigate cards; enter opens a detail pane with goal and acceptance
- [ ] Filter keys narrow by epic, sprint, and assignee
- [ ] The view refreshes automatically when the repo changes; q quits cleanly
- [ ] No write paths for status — the TUI is a viewer of the derived board
