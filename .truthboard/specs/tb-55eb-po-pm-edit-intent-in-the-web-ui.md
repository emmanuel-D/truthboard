---
id: tb-55eb
title: PO/PM create and edit stories in the web UI
owner: emmanuel
branch: '*/tb-55eb-*'
paths: [internal/web/**]
epic: po-experience
priority: 2
---

## Goal

As a PO who never opens a terminal, I can draft a story, refine its goal
and acceptance criteria, set its epic and priority — directly on the
board I already look at. The product rule survives intact and becomes
sharper: **the promise is editable, the proof is not.** Intent edits
write the markdown files (a plain git diff someone commits); statuses
remain computed with no endpoint able to touch them.

## Acceptance

- **Given** the board in a browser, **when** I click "New story," **then**
  a form (title, goal, acceptance, owner, epic, priority) creates the
  spec file and the card appears as `planned` on next refresh
- **Given** an existing card, **when** I open it and edit intent fields,
  **then** the file is rewritten and the diff shows in `git status`
- The page shows an "uncommitted intent changes" indicator when spec
  files differ from HEAD, so edits don't silently pile up
- Write endpoints exist only under `/api/specs` and can only touch spec
  files; a request attempting to set a status has no route to succeed —
  pinned by tests (method-guard test updated to the new contract)
- Every intent field remains equally editable via CLI/editor — the UI is
  a convenience, not the source of truth
