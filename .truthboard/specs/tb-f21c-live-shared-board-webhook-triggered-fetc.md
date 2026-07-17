---
id: tb-f21c
title: 'Live shared board: webhook-triggered fetch and browser push'
owner: emmanuel
branch: '*/tb-f21c-*'
paths:
    - internal/web/**
    - internal/lifecycle/**
epic: po-experience
priority: 3
---

## Goal

Blueprint §8.2 (Mode B): a centrally hosted board that updates the moment
a developer pushes, instead of waiting out the `--fetch` poll interval.
`truthboard ui --host` gains a `/webhook` endpoint (shared-secret
protected) that a GitHub/GitLab push webhook hits to trigger an immediate
fetch + re-derive; the page gets server push (SSE or WebSocket) so open
browsers update without reloading. Still read-only, still a single binary.

## Acceptance

- [ ] A forge push webhook to `/webhook` (with matching secret) triggers an immediate fetch and board refresh
- [ ] Requests with a missing or wrong secret are rejected and logged
- [ ] Connected browsers receive the update via server push — no manual reload, no fixed poll wait
- [ ] Polling `--fetch` keeps working unchanged when no webhook is configured
