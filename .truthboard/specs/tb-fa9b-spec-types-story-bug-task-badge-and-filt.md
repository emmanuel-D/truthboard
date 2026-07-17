---
id: tb-fa9b
title: 'Spec types: story, bug, task — badge and filter'
owner: emmanuel
branch: '*/tb-fa9b-*'
paths:
    - internal/spec/**
    - internal/web/**
epic: po-experience
priority: 1
---

## Goal

Bugs and stories are indistinguishable on the board. Add an optional
`type:` frontmatter field (`story` | `bug` | `task`, default `story`) —
pure intent, editable everywhere intent is. The web board renders a type
badge on each card and offers a type filter chip; the digest groups landed
work by type so a release note can separate fixes from features.

## Acceptance

- [ ] `type:` accepted in frontmatter, `spec new --type`, MCP create/update, and the web editor; unknown values are rejected with a clear error
- [ ] Cards show a type badge; a filter chip narrows the board by type
- [ ] Digest groups landed items by type (features vs fixes vs chores)
- [ ] Existing specs without `type:` default to story with no file changes required
