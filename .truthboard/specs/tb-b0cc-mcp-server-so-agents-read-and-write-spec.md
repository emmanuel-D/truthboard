---
id: tb-b0cc
title: MCP server so agents read and write specs natively
owner: emmanuel
branch: '*/tb-b0cc-*'
---

## Goal

`truthboard mcp` exposes the spec layer over MCP so Claude Code and other
agents stop shelling out: list specs, read a brief, create a spec, and get
the derived board as structured data. Specs stay the only writable surface —
the MCP server must not expose any way to set a status.

## Acceptance

- [x] MCP tools: list_specs, get_brief, create_spec, get_board (read-only)
- [x] Registers cleanly with Claude Code via `claude mcp add`
- [x] An agent can pick up a spec, work it, and the board reflects it with zero human input
