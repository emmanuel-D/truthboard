---
id: tb-7461
title: The story detail view shows every field the editor can write
owner: emmanuel
branch: '*/tb-7461-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 2
type: bug
---

## Goal

The detail modal has drifted behind the editor. The editor writes eleven
intent fields; the detail view renders some as chips, folds two into a
block headed "derived truth — computed, not editable", and drops declared
`repos` entirely unless the audit has already produced per-repo evidence.
So a PO can save a story that must land in three repos and then open it
and see no trace of that decision.

Reading a story must show everything a human declared about it. That is
the whole point of opening the card.

## Acceptance

- [x] Every field the editor writes — title, owner, type, priority, points, epic, sprint, repos, needs, scope, branch glob, hold, body — is visible in the detail view
- [x] A field nobody set reads "not set" instead of vanishing, so the reader can tell an empty field from a missing one
- [x] Declared repos show as intent (what done will require), separately from the derived per-repo landing evidence
- [x] The hold note is read from the story file itself, with git's contradiction supplied by the board, so a hold saved seconds ago shows before the next audit
- [x] Priority reads p1 · now / p2 · next / p3 · later — the same words the editor offers
- [x] Intent and derived truth sit in separate blocks: no editable field appears under the "computed, not editable" heading
- [x] The derived block also reports acceptance sign-off count, the repo a landing is in, and the story file path
- [x] `go build ./...` and `go test ./...` pass
