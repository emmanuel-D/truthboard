---
id: tb-2ed6
title: The Copilot docs name JetBrains, the one editor init cannot wire
owner: emmanuel
branch: '*/tb-2ed6-*'
paths:
    - README.md
epic: agent-loop
priority: 2
type: task
needs:
    - tb-8d9d
---

## Goal

As a developer running GitHub Copilot in IntelliJ IDEA (or any JetBrains
IDE), I want the README to tell me where my MCP config actually lives, so
I stop looking for a `.vscode/mcp.json` that my editor never reads.

tb-8d9d made `init --agents` write both `.mcp.json` and `.vscode/mcp.json`
and taught the README to say "Copilot". But `.vscode/mcp.json` is a VS Code
path: Copilot in JetBrains reads a **per-user, per-machine** file at
`~/.config/github-copilot/intellij/mcp.json` (Copilot status-bar icon →
Edit Settings → Model Context Protocol → Configure). Nothing committed can
reach it, so a JetBrains shop gets every part of adoption except the one
step that connects the server — and the README currently reads as if
`init --agents` covered them.

This is documentation, not code. There is no file in the repo we could
write to wire IntelliJ, and inventing one would be a lie. The honest fix
is a paste-ready snippet and the reason it has to be pasted — the same
shape as the coding-agent caveat that already sits in this section.

Two things make the JetBrains snippet *not* a copy of the VS Code one,
and both belong in the docs:

- The file is never committed, so the "no machine-local paths" rule that
  governs `.mcp.json` and `.vscode/mcp.json` does not apply here. For the
  `init --workspace` layout an absolute path to the hub is the right
  answer, not the relative `./hub` those committed files must use.
- `spawnWarning` (`internal/adopt/adopt.go`) exists for exactly this
  case: the IDE is GUI-launched, so a `truthboard` that lives only on a
  login shell's PATH will fail the connection silently.

The floor is unaffected and should be said plainly, because it is the
reassuring part: unlike Copilot's server-side coding agent, IntelliJ
Copilot commits locally, so the `commit-msg` hook fires, the `Spec:`
trailer nudge works, and the board is complete whether or not anyone
wires MCP at all.

## Acceptance

- [x] **Given** the README MCP section, **then** it carries a JetBrains /
  IntelliJ Copilot snippet next to the VS Code, Cursor, Codex and Gemini
  ones, naming the path `~/.config/github-copilot/intellij/mcp.json` and
  the in-IDE route to it (Copilot status bar → Edit Settings → Model
  Context Protocol → Configure)
- [x] **Given** that snippet, **then** it uses the `servers` key with
  `type: "stdio"` — Copilot's schema, the same as `.vscode/mcp.json` —
  so the two Copilot entries are visibly the same shape in different homes
- [x] **Given** a reader who just ran `init --agents`, **then** the README
  states outright that this file is per-user and per-machine and is
  therefore the one MCP config no truthboard command writes — no reader
  should conclude adoption already wired their IDE
- [x] **Given** the `init --workspace` layout, **then** the section says an
  absolute path to the hub is acceptable *here* precisely because the file
  is not committed, distinguishing it from the relative-path rule that
  governs `.mcp.json` and `.vscode/mcp.json`
- [x] **Given** a GUI-launched IDE, **then** the section points at the
  PATH/`spawnWarning` failure mode — a silently dead MCP connection — and
  its symlink fix, rather than leaving the reader to rediscover it
- [x] **Given** the existing coding-agent caveat, **then** the README
  distinguishes it from editor Copilot: JetBrains commits are local, so
  the hook and the trailer nudge do fire, and MCP wiring is a ceiling
  improvement, never a prerequisite for the board
- [x] **Given** the reader is in Copilot's ask or edit mode, **then** the
  docs note that MCP tools only surface in agent mode
- [x] No claim in the section is Claude-specific: model choice (Claude,
  GPT, whatever the picker offers) is stated to be irrelevant to MCP
  wiring, which is a property of the client
