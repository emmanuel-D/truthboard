---
id: tb-18df
title: Document that the MCP server works with any MCP client, not just Claude Code
owner: emmanuel
branch: '*/tb-18df-*'
epic: mcp-server
priority: 1
type: task
---

## Goal

The server in `internal/mcp` speaks standard MCP (stdio, JSON-RPC 2.0)
with nothing Claude-specific in the protocol, but the docs only show the
Claude Code registration command — so the question "does this work with
other AI tools?" keeps coming up. Answer it once, in the README: state
that any MCP-capable client works, and show how to register the server
in the common ones. Docs only; per-tool config writers in `adopt` wait
until a pilot on a non-Claude tool asks.

## Acceptance

- [ ] The README's MCP section states the server is tool-agnostic
      (standard stdio MCP), naming Claude Code as one client among others
- [ ] The README carries working registration snippets for at least
      Cursor, Codex CLI, and Gemini CLI alongside the Claude Code one
- [ ] The README notes that AGENTS.md is the cross-tool working
      agreement (CLAUDE.md merely imports it), so non-Claude agents get
      the agreement for free once their MCP config is in place
