---
id: tb-c298
title: 'CLAUDE.md carries the agreement: adopt writes it even when absent'
owner: emmanuel
branch: '*/tb-c298-*'
paths: [internal/adopt/**]
epic: agent-loop
priority: 1
---

## Goal

The retropixel3d-mono pilot (2026-07-16) proved that Claude Code never
loads AGENTS.md: a session started 30 minutes after adoption did a full
feature with no spec, no branch, no trailer — the working agreement simply
never entered context. Adopt currently treats CLAUDE.md as optional
("not present, skipped — AGENTS.md carries the agreement"), which is
exactly backwards for the most common agent.

Fix: `init --agents` always writes CLAUDE.md, creating it when missing.
The block imports the agreement via Claude Code's `@AGENTS.md` syntax so
the full text loads into context — one source of truth stays in AGENTS.md,
no duplicated prose to drift.

## Acceptance

- [ ] `truthboard init --agents` in a repo with no CLAUDE.md creates it
      with the truthboard block, and reports it as written
- [ ] The block contains an `@AGENTS.md` import line so Claude Code pulls
      the working agreement into context
- [ ] An existing CLAUDE.md keeps all its own content; only the
      marker-delimited block is added or replaced (re-adoption upgrades
      the old pointer text in place)
- [ ] Running adopt twice changes nothing and reports "already up to date"
