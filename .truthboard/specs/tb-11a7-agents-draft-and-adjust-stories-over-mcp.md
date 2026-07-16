---
id: tb-11a7
title: Agents draft and adjust stories over MCP
owner: emmanuel
branch: '*/tb-11a7-*'
paths:
    - internal/mcp/**
    - internal/spec/**
epic: agent-loop
priority: 1
---

## Goal

As an AI agent asked to "draft the backlog for this product idea," I can
create fully-formed stories (goal + acceptance, scope, epic, priority) and
later adjust them as understanding evolves — without a human touching the
terminal. Today `create_spec` only takes a title and leaves a placeholder
body, so an agent-drafted backlog is a list of empty promises. Intent is
the agent's to write; status remains nobody's to write.

## Acceptance

- [x] **Given** a product idea, **when** an agent calls `create_spec` with title,
  body (goal/acceptance markdown), owner, paths, epic, and priority,
  **then** one call produces a complete story file, visible on the next audit
- [x] **Given** an existing spec, **when** an agent calls `update_spec` changing
  any intent field (title, body, owner, branch, paths, epic, priority),
  **then** the markdown file is rewritten and the change is a plain git diff
- [x] **Given** any MCP request, **then** there is still no tool that writes a
  status — pinned by a test that tries and must fail
- [x] `update_spec` on an unknown id fails with a helpful message listing valid ids
