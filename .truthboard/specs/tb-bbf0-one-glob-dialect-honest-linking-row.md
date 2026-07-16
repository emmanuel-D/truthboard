---
id: tb-bbf0
title: One glob dialect, and a linking row that tells the whole truth
owner: emmanuel
branch: '*/tb-bbf0-*'
paths: [internal/audit/**, internal/web/**]
epic: po-experience
priority: 1
---

## Goal

Branch globs and spec `paths:` currently use two different glob dialects —
same syntax, different semantics (`**` works in one, not the other). And
the detail view shows only "Branch glob", implying that's the linking rule
when id-in-name and the commit trailer are stronger. One dialect everywhere;
one panel row that states all three signals.

## Acceptance

- [x] Branch globs use the same `**`-aware dialect as spec paths (e.g.
      `feat/**` links `feat/x/custom-name`)
- [x] The detail view replaces "Branch glob" with a "Linking" row: id-in-name,
      trailer, and glob — all three, in strength order
- [x] Existing default globs (`*/tb-xxxx-*`) keep matching exactly as before
