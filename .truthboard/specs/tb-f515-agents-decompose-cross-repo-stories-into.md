---
id: tb-f515
title: Agents decompose cross-repo stories into per-repo children
owner: emmanuel
branch: '*/tb-f515-*'
paths:
    - internal/mcp/**
    - internal/adopt/**
    - internal/audit/**
    - docs/**
epic: multi-repo
priority: 3
needs:
    - tb-c512
---

## Goal

`repos:` (tb-c512) is the mechanism; per-repo decomposition is the practice. The purest Truthboard shape for a cross-repo story is one provable landing per spec: a fat story becomes per-repo children under the same epic, ordered with `needs:`, each honestly derivable on its own. A PO on the phone should never need to know a story touches three repos — the agent picking it up does the splitting.

Teach the loop to do that:

- The adoption agreement (AGENTS.md / CLAUDE.md written by `truthboard adopt`) gains guidance: when a brief's acceptance clearly spans multiple workspace repos, split it into per-repo stories over MCP (`create_spec` + `needs:`), link them to the original via the epic, and narrow the original's `repos:`/scope accordingly.
- `get_brief` on a spec with multi-repo scope includes the workspace repo list and the split-or-declare choice in its linking instructions.
- `next_spec` respects cross-repo `needs:` ordering so an agent in the `web` repo is not handed a story blocked on unlanded `api` work (readiness is already derived; verify it holds across repos).

## Acceptance

- [ ] `truthboard adopt` writes decomposition guidance into the working agreement when a workspace manifest exists.
- [ ] `get_brief` for a multi-repo-scoped spec surfaces the workspace repos and instructs the agent to split or declare `repos:` explicitly.
- [ ] An agent following the brief can create per-repo children over MCP with `needs:` ordering, and the epic rollup shows combined progress.
- [ ] `next_spec` never hands out a story whose cross-repo `needs:` are not yet done.
- [ ] Docs: a worked example — one phone-created story becoming two per-repo children, board deriving each to done.
