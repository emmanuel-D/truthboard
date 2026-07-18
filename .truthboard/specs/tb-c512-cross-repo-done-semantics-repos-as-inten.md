---
id: tb-c512
title: 'Cross-repo done semantics: repos as intent, per-repo evidence'
owner: emmanuel
branch: '*/tb-c512-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/web/**
    - internal/tui/**
    - docs/**
epic: multi-repo
priority: 2
needs:
    - tb-8a79
---

## Goal

Git cannot prove the absence of work it never knew was intended: a story touching `api` and `web` looks done the moment the trailer lands in `api`, even though the web half was never started. This is the one place multi-repo needs new *declared intent*.

Add an optional `repos:` frontmatter field: a spec declaring `repos: [api, web]` is **done only when its trailer has landed on the integration branch of every declared repo**. Everything else generalizes the existing rules across the repo set:

- Active work anywhere outranks done (current `deriveSpecStatus` rule, applied over all repos).
- A revert of the spec's landed work in any repo, or red CI on any landing commit, flips it regressed.
- Specs without `repos:` keep the workspace-wide "landed anywhere" semantics from tb-8a79 — the field is opt-in, like paths.

Evidence is where this must shine: a partially-landed spec shows per-repo evidence, e.g. `api ✓ landed 3d ago · web — no branch yet`, never a mute "in-progress". Honest per-repo chips are the multi-repo version of the honesty rule.

## Acceptance

- [ ] `repos:` parses from spec frontmatter and round-trips through `update_spec` (MCP + CLI + web editor).
- [ ] A spec with `repos: [a, b]` whose trailer landed only in `a` derives in-progress (or stalled/planned per branch state in `b`), never done; landing in both derives done.
- [ ] A `repos:` entry not present in the workspace manifest is surfaced as a loud audit finding, not silently ignored.
- [ ] Evidence strings and the web/TUI detail view show per-repo landing state for specs with `repos:`.
- [ ] Revert or red CI in any declared repo flips the spec to regressed, evidence naming the repo.
- [ ] Docs explain when to declare `repos:` versus splitting into per-repo child stories under an epic.
