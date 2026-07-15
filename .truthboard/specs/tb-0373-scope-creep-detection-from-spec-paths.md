---
id: tb-0373
title: Scope-creep detection from spec paths
owner: emmanuel
branch: '*/tb-0373-*'
paths: [internal/audit/**]
---

## Goal

Use the optional `paths:` frontmatter as a declared scope: when a
spec-linked branch's diff touches files far outside those globs, surface a
drift finding. Catches "while I was in there" commits before review does.

## Acceptance

- [ ] A linked branch whose diff is >50% outside the spec's paths yields a scope-creep drift finding
- [ ] Specs without `paths:` are never flagged (opt-in only)
- [ ] Finding names the top offending directories, not every file
