---
id: tb-84a7
title: The teammate who only lives in the forge can see the board
owner: emmanuel
branch: '*/tb-84a7-*'
paths:
    - internal/forge/**
    - internal/audit/**
    - cmd/truthboard/**
    - docs/**
epic: multi-repo
priority: 3
type: story
---

## Goal

The forge boundary is one-way. `tb-711f` and `tb-fd8f` taught the board
to *read* GitHub and GitLab — PRs, CI, claims — and nothing has ever
gone the other direction. So the tool serves the person running it and
the agents it wires, and stops there. The reviewer who opens a PR, the
colleague who lives in Issues, the stakeholder with a browser and no Go
toolchain: none of them can see what a story promised, and none of them
can raise one without being handed a terminal.

The read-only web board (`ui --host`) is the current answer, and it
needs a machine, a port and a URL somebody keeps alive. The forge is
already where those people are.

Constraint that decides the design: the markdown files stay the source
of truth. A mirrored issue is an *output*, like the digest — it may be
rewritten from the specs at any time, and nothing on the forge may ever
become the thing the board derives from. Editing a mirrored issue must
therefore not silently lose the edit: either it is reflected back into
the spec deliberately, or the issue says plainly that it is a mirror and
where the original lives.

## Acceptance

- [ ] `truthboard mirror` publishes stories as issues on the configured forge, via `gh`/`glab` as the existing adapters do, with the derived status, acceptance checklist and tick state in the body
- [ ] Re-running it updates the issues it already created rather than opening duplicates, and closes those whose stories derived done
- [ ] Each mirrored issue states that it is a mirror and names the spec file it came from, so nobody edits it believing it is the source
- [ ] Mirroring is opt-in and dry-runnable: `--dry-run` shows exactly what would be created, updated and closed, and writes nothing
- [ ] The mapping between spec ids and issue numbers survives a fresh clone — it is derived or committed, never held in an untracked local file
- [ ] A forge that is unreachable, unauthenticated or rate-limited fails with a message that names the cause, and never leaves the specs half-mirrored without saying so
- [ ] Tokens and credentials never reach the logs
- [ ] Nothing on the forge feeds a derived status: statuses still come from git alone
- [ ] `go test ./...` passes
