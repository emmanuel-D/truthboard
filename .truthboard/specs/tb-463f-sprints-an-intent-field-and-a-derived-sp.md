---
id: tb-463f
title: 'Sprints: an intent field and a derived sprint digest'
owner: emmanuel
branch: '*/tb-463f-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/mcp/**
    - internal/web/**
    - internal/report/**
    - cmd/truthboard/**
    - README.md
epic: po-experience
priority: 1
---

## Goal

Teams that plan in sprints have no way to say "this story is in sprint
s12" and no answer to the two sprint questions: how far along is the
sprint, and what actually landed in it. Both answers exist in the repo —
what's missing is the grouping.

A sprint is *intent* (a slug on the spec, like epic), so it belongs in
frontmatter and is editable everywhere intent is: `spec new --sprint`,
MCP create/update, the web editor. The sprint *digest* is derived: per
sprint, stories done vs total and what is still open, each story's status
coming from git exactly as before. No dates, no sprint files, no
"activate sprint" command — a sprint completes when its stories land,
and there is nothing to type.

Surfaces: a sprint rollup in the audit result (terminal, markdown, JSON —
so the weekly Action issue tells the sprint story too), a sprints panel
plus card tags and filter chips on the web board.

## Acceptance

- [ ] `sprint` is a frontmatter field, settable via `spec new --sprint`,
      MCP create_spec/update_spec, and the web editor
- [ ] the audit result carries a per-sprint rollup (done/total plus open
      stories with their derived statuses); terminal and markdown reports
      render it; sprintless repos see nothing new
- [ ] the board shows a sprint tag on cards, sprint filter chips, and a
      sprint progress panel
- [ ] statuses inside the rollup are the same derived statuses as
      everywhere else — no sprint-level status exists to set
- [ ] README documents sprints in the spec-mode section
