---
id: tb-a4ab
title: Init tells you when the hub is not a git repository yet
owner: emmanuel
branch: '*/tb-a4ab-*'
paths: [internal/adopt/**, internal/gitrepo/**, cmd/truthboard/**]
epic: agent-loop
priority: 1
---

## Goal

A clean-room run of a real hub setup — released v0.8.0 binary, no dev
build — found the one onboarding hole `truthboard init` still has. Single
repos are already git repos when you adopt them, so nobody hit this; a
**multi-repo hub is the one repo people create as an empty directory by
hand**, and `init --workspace` scaffolded it in silence. The manifest,
`.mcp.json`, `AGENTS.md` and `CLAUDE.md` all landed, the command exited 0,
and the closing "Next:" block pointed at `truthboard spec new` — straight
into a wall, because every derived status starts from git history.

The wall itself was worse than the silence: `truthboard audit` in that
directory printed

    truthboard: git for-each-ref refs/heads refs/remotes/origin
    --format=%(refname)|%(objectname)|%(committerdate:iso8601-strict):
    fatal: not a git repository

— the name of an internal plumbing invocation, and nothing the reader can
act on. Exit codes were already correct (1); only the message was wrong.

Fix both ends: init reports the missing repository and leads its next steps
with `git init`, and the "not a git repository" failure reads as guidance
from every command that derives anything. Warn-only, following the
`spawnWarning` precedent from [[tb-4b90]] — the wiring init writes is
correct on disk either way, and `git init` stays the adopter's call.

## Acceptance

- [ ] `truthboard init --workspace` in a non-git directory prints a loud
      warning naming the gap and a copy-paste `git init` fix, and its
      "Next:" block leads with `git init`
- [ ] The same command in a git repo stays quiet — no warning, action log
      unchanged
- [ ] The warning never fails init: exit stays 0 and every file
      (`.mcp.json`, `AGENTS.md`, `CLAUDE.md`, manifest) is still written
- [ ] `truthboard audit` outside a work tree names the directory and says
      to run `git init`, and never prints the internal git command
- [ ] Git failures inside a real repo keep their command context — the
      translation is scoped to "not a git repository" alone
