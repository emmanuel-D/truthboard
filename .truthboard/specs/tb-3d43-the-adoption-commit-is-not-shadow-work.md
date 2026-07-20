---
id: tb-3d43
title: The adoption commit is not shadow work
owner: emmanuel
branch: '*/tb-3d43-*'
paths: [internal/audit/**]
epic: agent-loop
priority: 1
---

## Goal

Installing truthboard on the LetTalk hub for real surfaced a first-impression
bug: the commit that adopts truthboard is reported as **drift on the very
board it creates**. The hub's `Track work with truthboard` commit — manifest,
`.mcp.json`, `AGENTS.md`, `CLAUDE.md` — landed in the shadow-work section of
its own first audit.

[[tb-6e13]] exempted commits touching only `.truthboard/`, on the reasoning
that writing a story is intent rather than work. The adoption commit is the
same kind of thing but has a wider fileset, so it fell straight through the
exemption. It also *must* land directly on the integration branch: there is
no board to open an MR against yet, which is precisely the shape shadow-work
detection looks for.

Every new adopter meets this on their first audit — the board's opening
statement accuses their setup of being drift, in a product whose whole claim
is that its findings are trustworthy.

Widen the exemption from `.truthboard/` to the fileset truthboard itself
writes and owns: `.truthboard/**`, `.mcp.json`, `AGENTS.md`, `CLAUDE.md`. A
commit confined to those changes how work is tracked, never the product.
Mixed commits stay shadow work — smuggling code in beside intent is exactly
what the finding should catch.

## Acceptance

- [ ] A commit touching only governed files (`.truthboard/**`, `.mcp.json`,
      `AGENTS.md`, `CLAUDE.md`) is never reported as shadow work
- [ ] A commit mixing governed files with product code stays shadow work
- [ ] The LetTalk hub's own adoption commit no longer appears in its audit,
      and the genuine spoke findings are untouched
