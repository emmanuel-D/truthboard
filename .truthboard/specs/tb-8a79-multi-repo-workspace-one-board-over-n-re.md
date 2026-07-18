---
id: tb-8a79
title: 'Multi-repo workspace: one board over N repositories'
owner: emmanuel
branch: '*/tb-8a79-*'
paths:
    - internal/workspace/**
    - internal/audit/**
    - internal/web/**
    - cmd/truthboard/**
    - docs/**
epic: multi-repo
priority: 1
---

## Goal

Real projects span 2–10 repositories, but Truthboard today hardwires one repo: specs, audit, sync loop, and board all assume a single git history. This story makes the hub-repo model real: **intent lives in one hub repo** (the repo carrying `.truthboard/`), and **proof is gathered from N spoke repos** declared in a versioned workspace manifest.

Design decisions (from the 2026-07-18 brainstorm):

- A manifest at `.truthboard/workspace.yml` in the hub lists spoke repos — name, remote, integration branch. The repo list is itself intent: versioned, diffable, editable like any spec. A missing manifest means today's single-repo behavior, untouched (a single-repo setup is just a workspace of one).
- The audit loops over the workspace: units and spec evidence gain a repo dimension. `Unit` and `SpecStatus` carry a `Repo` field; the board renders `api:feature/tb-1234-…`. Trailer/branch/glob linking works unchanged per repo — the id namespace is global because intent is central.
- The board server keeps a clone per spoke (bare/mirror is fine — the audit is read-only) and runs one sync loop per repo; sync-freshness headers become per-repo so a stale spoke can never masquerade as a quiet one.
- No status-semantics changes in this story: a spec is done exactly as today (trailer landed anywhere, active work anywhere outranks done). Multi-repo done semantics are the follow-up story.
- Scope-creep paths may carry an optional repo prefix (`api:internal/auth/**`); unprefixed paths keep matching the hub.

## Acceptance

- [ ] `.truthboard/workspace.yml` (repos: name, remote, integration branch) is parsed and validated; absent manifest preserves single-repo behavior with zero config.
- [ ] `truthboard audit` / `get_board` over a workspace returns units and spec statuses from every declared repo, each tagged with its repo name.
- [ ] A spec whose trailer lands in a spoke repo (not the hub) flips to done on the board, with evidence naming the repo.
- [ ] The board server maintains and fetch-syncs a clone per spoke; per-repo sync freshness is reported on board responses.
- [ ] Web board and TUI show the repo tag on branch units and in spec evidence.
- [ ] Docs: a multi-repo quick start (hub setup, manifest, spoke adoption pointing agents at the hub/shared board).
