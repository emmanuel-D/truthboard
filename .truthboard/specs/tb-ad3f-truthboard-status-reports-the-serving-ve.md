---
id: tb-ad3f
title: truthboard status reports the serving version and flags a stale board
owner: emmanuel
branch: '*/tb-ad3f-*'
paths: [internal/lifecycle/**, cmd/truthboard/**]
epic: dev-experience
priority: 2
---

## Goal

A detached board pins the binary it started with. Install a new release and
the board keeps serving the old one — old audit logic, old UI — until it is
stopped and re-detached. The web page already says which version it serves
(`X-Truthboard-Version` on `/api/board`, rendered by the title and footer),
but the page is exactly where you are *not* looking after an install.
`truthboard status` — the command whose entire job is "what is my board
doing?" — reports URL, pid, uptime, fetch interval and sharing, and never
the version (`lifecycle.go:273`).

This bit for real on 2026-07-20. Both boards had been running 13h across a
v0.8.0 → v0.8.1 → v0.8.2 sequence, still serving pre-fix audit logic while
the CLI beside them reported v0.8.2. Worse, the binary they were running
had been **deleted** — they survived on an unlinked inode, so `ps` showed a
path that no longer existed and they would have failed to restart from it.
Nothing in `status` hinted at any of this; it was found by reading `ps`.

`status` should answer the staleness question directly: report the serving
version, and when it differs from the binary answering the command, say so
and name the fix (`truthboard stop` then re-issue `ui --detach`). The
version is one HEAD request against a URL `status` already holds, and the
board is local — but a board that does not answer must degrade to today's
line, never hang or error, because "unreachable" is itself worth seeing.

## Acceptance

- [ ] `truthboard status` on a board serving the running binary's version
      includes that version in its line
- [ ] A board serving a different version than the CLI is called out as
      stale, naming both versions and the stop/`ui --detach` fix
- [ ] A running board that does not answer the request still produces the
      current status line, marked unreachable — never a hang or an error
- [ ] A board whose binary was deleted or replaced on disk is reported as
      stale on the version comparison alone, with no dependence on the
      executable path still resolving
- [ ] `no detached board` and `stale state cleaned up` paths are unchanged
