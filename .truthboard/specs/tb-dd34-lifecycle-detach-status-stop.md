---
id: tb-dd34
title: 'Board lifecycle: ui --detach, status, stop'
owner: emmanuel
branch: '*/tb-dd34-*'
paths: [cmd/truthboard/**, internal/lifecycle/**, internal/web/**]
epic: po-experience
priority: 1
---

## Goal

A PM's board should be a fixture, not a terminal tab. `truthboard ui
--detach` runs the board in the background for this repo; `truthboard
status` says what's running where; `truthboard stop` ends it. Runtime
state lives under `.git/` — per-repo, never committed, no system
services, no root.

## Acceptance

- [x] `truthboard ui --detach` starts the board in the background, prints
      the URL, and survives the terminal closing (own session)
- [x] `truthboard status` reports url, pid, and uptime of a running board —
      and cleans up stale state from a crashed one
- [x] `truthboard stop` terminates the detached board and cleans state;
      a second stop says nothing is running
- [x] A second `--detach` while one runs refuses, pointing at the live URL
- [x] Runtime state lives inside the git dir (never committable); Windows
      says "not supported yet" instead of half-working
