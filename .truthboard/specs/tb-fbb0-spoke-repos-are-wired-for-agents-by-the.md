---
id: tb-fbb0
title: Spoke repos are wired for agents by the same command that declares them
branch: '*/tb-fbb0-*'
paths:
    - internal/adopt/**
    - internal/workspace/**
    - cmd/truthboard/spec_cmds.go
    - docs/multi-repo.md
    - README.md
epic: agent-loop
priority: 1
---

## Goal

`truthboard init --workspace` declares spokes in the hub manifest but wires
only the hub for agents: `.mcp.json`, `.vscode/mcp.json`, `AGENTS.md`,
`CLAUDE.md` and the commit-msg nudge all land in the hub, and every spoke —
the repos where agents actually write code — gets nothing. The result is the
worst arrangement available: a hub that looks perfectly set up while an agent
opened in `api/` has no board at all, no working agreement, and no trailer
nudge, so its commits land silently and surface later as shadow work.

Wire every spoke that has a local checkout, from the same command, so
multi-repo setup costs exactly what single-repo setup costs.

## Acceptance

- [ ] `init --workspace` wires each declared spoke that resolves to a local
      checkout: MCP registration (both files), the working agreement, and the
      commit-msg nudge when `--hooks` is given.
- [ ] A spoke's MCP entry carries the hub as its argument — `["mcp", "../hub"]`
      computed relative from spoke to hub, never an absolute path (the file is
      committed and shared). A bare `["mcp"]` in a spoke would serve a repo with
      no `.truthboard/`.
- [ ] The spoke's agreement says where intent lives: work and trailer belong in
      this repo, specs and the board live in the hub, ids are global.
- [ ] Spoke identity is verified before writing — the existing `Resolve` check
      that refuses a path holding a checkout of some other repository.
- [ ] A spoke with only a `remote:` and no local copy is skipped **by name**,
      with the command to run once it is checked out. Adoption never clones.
- [ ] Every write is printed per spoke, in the existing log style.
- [ ] Re-running changes nothing: same idempotent `upsertBlock` / `registerMCP`
      / `installHook` machinery, and a spoke's pre-existing `.mcp.json` servers,
      `AGENTS.md` prose and commit-msg hook all survive.
- [ ] `--no-spokes` opts out and touches no repo but the hub.
- [ ] `docs/multi-repo.md` drops the manual spoke-adoption instructions, and its
      reference to a `truthboard adopt` command that does not exist.
