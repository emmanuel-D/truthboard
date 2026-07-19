---
id: tb-9aa1
title: 'truthboard init --workspace: scaffold a hub in one command'
owner: emmanuel
branch: '*/tb-9aa1-*'
paths:
    - cmd/truthboard/**
    - internal/workspace/**
    - internal/adopt/**
    - docs/**
epic: dev-experience
priority: 2
---

## Goal

Setting up a multi-repo hub today is hand-editing YAML from docs. One command should scaffold it: `truthboard init --workspace api=git@github.com:acme/api.git web=git@github.com:acme/web.git` writes a validated `.truthboard/workspace.yml` (name=remote pairs; optional `--path name=../checkout` for local spokes), creates `.truthboard/specs/`, and runs the same agent wiring as `init --agents` — which then already includes the multi-repo decomposition guidance (tb-f515) because the manifest exists. Re-running with new pairs merges into the existing manifest rather than clobbering it, and every name goes through the same validation the manifest loader enforces (reserved `hub`, name grammar).

## Acceptance

- [ ] `truthboard init --workspace name=remote [name=remote ...]` writes a valid manifest and reports each declared spoke.
- [ ] Invalid names (grammar, reserved `hub`, duplicates) fail before anything is written.
- [ ] Re-running merges new spokes into an existing manifest; existing entries are never silently rewritten.
- [ ] The written hub passes `truthboard audit` immediately (spokes show as loud unreadable findings until the server clones or paths exist — expected, documented in output).
- [ ] docs/multi-repo.md quick start leads with the one-command path.
