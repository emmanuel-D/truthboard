---
id: tb-fd8f
title: GitLab adapter via glab for claims-vs-proof
owner: emmanuel
branch: '*/tb-fd8f-*'
paths: [internal/forge/**]
---

## Goal

Bring GitLab-hosted repos (like retropixel3d-mono) up to full coverage:
fetch issues/MRs read-only via `glab`, feeding the same claims-vs-proof
findings and in-review upgrades the GitHub adapter provides. Same silent
degradation when `glab` is missing.

## Acceptance

- [ ] On a GitLab repo with `glab` authed, branches with open MRs show in-review
- [ ] Claims section works: stale assigned issues, done-but-open, abandoned MRs
- [ ] Without `glab`, output is byte-identical to today's pure-git audit
