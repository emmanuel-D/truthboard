---
id: tb-b5a1
title: Adoption says so when it writes a file the repo will ignore
branch: '*/tb-b5a1-*'
paths:
    - internal/adopt/**
    - docs/multi-repo.md
    - README.md
epic: agent-loop
priority: 2
---

## Goal

Adoption reports success on a write that can never be shared. Wiring
`lettalk-server` logged `.vscode/mcp.json: registered the truthboard MCP
server` while that repo's `.gitignore:30` excludes `.vscode/*` — the file
exists on that laptop, works for that developer, and cannot be committed, so
every teammate and every fresh clone gets a repo with no MCP server for VS
Code or Copilot. The log was true about the write and misleading about the
outcome.

This is the same failure shape the unwired-spoke finding exists for: a repo
that looks adopted while the agents working in it have no board. Here the
gap is invisible even to `truthboard audit`, because the file is present on
the machine doing the auditing.

Adoption should say what it knows: this file is written, and this repo will
not keep it.

## Acceptance

- [ ] Each file adoption writes is checked against the repo's ignore rules,
      and an ignored one is reported in the same log, next to the line
      claiming it was written — not as a separate summary a reader can miss.
- [ ] The warning names the rule that does it: the ignore file and line, as
      `git check-ignore -v` reports them, so the reader can go fix the
      actual cause rather than hunt for it.
- [ ] The suggested exception is one that **works for that pattern shape**.
      Git cannot re-include a file whose parent directory is excluded, so
      `.vscode/` needs `!.vscode/`, `.vscode/*` and `!.vscode/mcp.json`,
      while a bare `.vscode/*` needs only the last. A fix that silently does
      nothing would be worse than no suggestion at all.
- [ ] A file that is **already tracked** is never warned about: ignore rules
      do not apply to tracked files, and crying wolf over a committed
      `.mcp.json` teaches people to ignore the warning that matters.
- [ ] Warn-only, exit code unchanged — same doctrine as the spawn and
      not-a-git-repo warnings. Adoption's job is to write the files and tell
      the truth about them, never to edit someone's `.gitignore`.
- [ ] The wiring drift finding tells the same truth: a spoke whose wiring is
      present locally but ignored is reported, because a teammate's clone of
      that spoke has no board. Its own repo, its own rules — the hub's
      `.gitignore` says nothing about a spoke's.
- [ ] A repo with no ignore rules on these paths sees no change at all, and
      a directory that is not a git repository is not probed for them.
- [ ] Documented where the wiring is documented.

## Notes

Found while wiring the LetTalk spokes on 2026-08-08. `lettalk-server` is the
live instance: `.gitignore:30` is `.vscode/*`, so its `.vscode/mcp.json` is
the shape that needs only the single negation, while a repo ignoring
`.vscode/` outright needs all three lines.
