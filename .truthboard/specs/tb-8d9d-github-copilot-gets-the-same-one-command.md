---
id: tb-8d9d
title: GitHub Copilot gets the same one-command wiring as everyone else
owner: emmanuel
branch: '*/tb-8d9d-*'
paths:
    - internal/adopt/**
    - internal/audit/**
    - .vscode/**
    - README.md
epic: agent-loop
priority: 1
---

## Goal

As a developer in a GitHub Copilot shop, I want `truthboard init --agents`
to wire *my* tool too. Today it writes `.mcp.json` with the `mcpServers`
key — Claude Code's convention — and the README hand-holds Cursor, Codex
CLI and Gemini CLI while never once saying the word "Copilot". Copilot in
VS Code reads `.vscode/mcp.json` and uses a *different* top-level key
(`servers`), so the largest AI-tool population in the enterprise is the
one group that has to hand-write config from a schema we never document.

The floor already holds — branch names and `Spec:` trailers link Copilot's
commits with zero integration, and Copilot reads `AGENTS.md` like every
other tool. This is about the ceiling: point-and-go MCP, plus honesty
about the one case we genuinely cannot wire (the server-side coding agent
configures MCP in GitHub repo settings, not a committed file, and its
commits never touch the local `commit-msg` hook).

## Acceptance

- [x] **Given** a fresh repo, **when** I run `truthboard init --agents`,
  **then** it writes `.vscode/mcp.json` registering the truthboard server
  under the `servers` key with `type: "stdio"`, alongside the existing
  `.mcp.json`
- [x] **Given** a `.vscode/mcp.json` that already has other servers,
  **then** they are preserved and only the `truthboard` entry is added —
  same merge contract as `registerMCP`
- [x] **Given** truthboard is already registered there, **when** I re-run
  init, **then** the file is untouched and the step reports "already
  registered" — idempotent like every other adopt step
- [x] **Given** either file, **then** neither pins a machine-local path:
  both register the bare `truthboard mcp`, and the workspace layout's
  `mcp ./hub` argument stays a documented manual edit for both — the files
  are committed and shared
- [x] **Given** an existing `.vscode/mcp.json` that is not valid JSON,
  **then** init fails loudly naming the file, never silently overwriting it
- [x] The README MCP section documents the Copilot/VS Code snippet next to
  Cursor/Codex/Gemini, and states the two coding-agent caveats: MCP is
  configured in GitHub repo settings, and the local trailer nudge never
  fires for server-side commits (branch-name linking covers it)
