---
id: tb-6e13
title: 'Remote intent editing: create stories from a shared board, agents pick them up at home'
owner: emmanuel
branch: '*/tb-6e13-*'
paths:
    - internal/web/**
    - cmd/truthboard/**
    - docs/**
epic: po-experience
priority: 2
---

## Goal

Today a board served beyond loopback is read-only because there is no
auth story. The wanted workflow: from a phone or any browser, open the
shared board, create or edit a story/bug/task; the server commits the
intent change to `.truthboard/specs/` in its clone and pushes it to
origin. Back on the PC, `git pull` (or the agent's own fetch) brings the
new specs, and `truthboard next` / `next_spec` hands them to the agent —
the round trip from idea on the road to agent working at home closes
with zero copy-paste.

Statuses stay derived — this opens intent (the promise) only, never
status (the proof).

## Acceptance

- [ ] An edit token (`--edit-token` / `TRUTHBOARD_EDIT_TOKEN`) arms intent writes on a non-loopback board; without it the board stays read-only exactly as today.
- [ ] Authenticated create/update of specs on a shared board commits to the server clone with a clear author/message and pushes to origin; push failures surface in the UI, not silently.
- [ ] Unauthenticated write attempts still get the read-only 403; the token never gates reads.
- [ ] The deploy guide documents the token setup and the phone → origin → `truthboard next` round trip.
- [ ] Concurrent-edit safety: the server pulls/rebases intent before committing, and a conflict returns an actionable error instead of corrupting specs.
