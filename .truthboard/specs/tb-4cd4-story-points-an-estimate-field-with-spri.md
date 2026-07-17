---
id: tb-4cd4
title: 'Story points: an estimate field with sprint and epic rollups'
owner: emmanuel
branch: '*/tb-4cd4-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/web/**
epic: po-experience
priority: 1
---

## Goal

The blueprint promised estimation ("rolling up metrics to Sprints") but
specs have no `points:` field, so there is no velocity math anywhere. Add
optional integer `points:` frontmatter — intent, editable via `spec new
--points`, MCP create/update, and the web editor. Derived rollups follow:
the sprint digest reports points done vs points planned, and epic groupings
on the board show their point totals.

## Acceptance

- [ ] `points:` is accepted in frontmatter, `spec new --points`, MCP create_spec/update_spec, and the web story editor
- [ ] Sprint digest shows points done vs total alongside story counts
- [ ] Board epic groups (and stat tiles) show point totals
- [ ] Specs without points keep working; rollups count them as unestimated rather than zero-point
