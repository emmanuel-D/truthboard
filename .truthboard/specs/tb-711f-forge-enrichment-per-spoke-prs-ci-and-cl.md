---
id: tb-711f
title: 'Forge enrichment per spoke: PRs, CI, and claims from every workspace repo'
owner: emmanuel
branch: '*/tb-711f-*'
paths:
    - internal/forge/**
    - internal/audit/**
    - internal/workspace/**
    - docs/**
epic: multi-repo
priority: 2
---

## Goal

Forge enrichment is hub-only today (a deliberate tb-8a79 guard: matching a spoke branch against a hub PR of the same name would be a false claim). In a real workspace every spoke has its own PRs, issues, and CI — the board should show `in-review` for a spoke branch with an open PR, run claims-vs-proof per repo, and let red CI on a spoke landing flip a `repos:` story regressed (tb-c512 wired the LandedRepo guard for exactly this moment).

Design sketch: `forge.Fetch` per resolvable spoke (gh/glab already auto-detect from the repo's remote), enrichment loop keyed by `Unit.Repo`/`RepoLanding.Repo`, claims tagged with the repo name. The 15s forge cache interval may need to be per-repo. A spoke without an authed forge CLI degrades to git-only silently-but-visibly (a note, not an error — same honesty rule as sync freshness).

## Acceptance

- [ ] A spoke branch with an open PR derives in-review, evidence naming PR and repo.
- [ ] CI red on a spoke landing flips the spec (including `repos:` specs) to regressed with the repo named.
- [ ] Claims-vs-proof runs per spoke; claim subjects carry the repo name.
- [ ] A spoke whose forge is unreachable/unauthed shows a visible note and keeps git-only derivation.
- [ ] docs/multi-repo.md "Current limits" section updated (the hub-only limit falls).
