---
id: tb-44da
title: Digest tells the story from specs, not commit subjects
owner: emmanuel
branch: '*/tb-44da-*'
paths: [internal/audit/**, internal/report/**, internal/web/**]
epic: po-experience
priority: 2
---

## Goal

As a stakeholder reading the weekly digest, I want to see *what promises
were kept* — "Tennis M0 prototype landed" — not a list of commit
subjects I can't map to anything. The digest should lead with specs whose
work landed in the window, told by their titles, with raw commits demoted
to a supporting "also landed" list.

## Acceptance

- [x] **Given** a spec whose landing commit falls inside the digest window,
  **then** the digest leads with "✓ <spec title> (<id>) — landed <date>"
  in term, md, and web UI
- [x] Commits not attributable to any spec still appear, under an
  "also landed" subsection — nothing is hidden
- [x] **Given** a repo with no specs, **then** the digest renders exactly as
  today (pure commit list)
- [ ] The weekly GitHub Action issue picks the new format up with no
  workflow change
