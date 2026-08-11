---
id: tb-2c89
title: Adoption spots a JetBrains IDE and hands over the one config it cannot write
owner: emmanuel
branch: '*/tb-2c89-*'
paths:
    - internal/adopt/**
    - README.md
epic: agent-loop
priority: 2
type: story
needs:
    - tb-2ed6
---

## Goal

As someone adopting truthboard from IntelliJ IDEA, I want `init --agents`
to notice that I work in a JetBrains IDE and hand me the MCP config it
cannot write, so the one gap in my wiring is something I was told about
rather than something I discover when an agent works my repo with no board.

tb-2ed6 put the JetBrains snippet in the README. That helps the person who
reads the README. It does nothing for the person who runs one command,
watches ten green lines scroll past, and reasonably concludes they are
wired — which is exactly the failure `spawnWarning` and `ignoreWarning`
exist to prevent. A `.idea/` directory in the repo is a strong, cheap
signal, and adoption is already the moment we tell people the truth about
their wiring.

**Warn, never write.** Copilot's JetBrains config is
`~/.config/github-copilot/intellij/mcp.json` — global to the machine,
shared by every project the IDE opens. Everything adoption writes today
lands inside the repo it was pointed at: reviewable in the adoption diff,
revertible with git, committed for the team. Reaching into `$HOME` to
change config for projects truthboard was never invited into is a
different act, and it is not this story. This story is the doctor,
following `spawnWarning` exactly: name the problem, name the file, print
the fix, exit successfully.

Two facts make detection an imperfect signal, which is another argument
for warning rather than acting: `.idea/` says *IntelliJ*, not *which
assistant* — Copilot, JetBrains AI Assistant and Junie each keep their own
MCP config — and `.idea/` is very often gitignored, so the check must look
at the disk, not at git.

Out of scope, deliberately: an opt-in `--jetbrains` flag that writes the
global file. That design is blocked on an unanswered question — what
working directory the IntelliJ Copilot plugin spawns a stdio MCP server
in. If it is the open project's root, one global entry with a bare
`truthboard mcp` serves every project and the flag is attractive; if it is
`$HOME` or the IDE's install directory, a single global slot cannot serve
N repos and the flag would be writing a config that works for exactly one
of them. Answer that before speccing the write.

## Acceptance

- [ ] **Given** a repo containing a `.idea/` directory, **when** I run
  `truthboard init --agents`, **then** the log carries a warning naming
  `~/.config/github-copilot/intellij/mcp.json` as the one MCP config no
  truthboard command writes, and prints the `servers`/`type: "stdio"`
  snippet to paste there
- [ ] **Given** no `.idea/` anywhere in the repo, **then** nothing about
  JetBrains is printed — silence is the default, and this warning never
  becomes noise every adopter learns to skim past
- [ ] **Given** `.idea/` is listed in `.gitignore`, **then** the warning
  still fires: the check reads the filesystem, because an ignored `.idea/`
  is still a developer using IntelliJ
- [ ] **Given** the global config already registers a truthboard server,
  **then** the warning does not fire — a wired machine is not nagged on
  every re-run, the same courtesy `spawnWarning` extends
- [ ] **Given** the global config is absent, unreadable, or malformed JSON,
  **then** the warning fires and adoption still succeeds: this step can
  never fail a wiring, and never rewrites a file it could not parse
- [ ] **Given** a workspace hub with spokes, **when** adoption wires a
  spoke that carries `.idea/`, **then** that spoke's own step log carries
  the warning — reported next to the repo it is true about, like
  `ignoreWarning`, not as a trailing summary
- [ ] **Given** the workspace layout, **then** the printed snippet uses an
  absolute path to the hub rather than `./hub`, because that file is not
  committed and the relative-path rule does not govern it
- [ ] The detection is a pure function over a repo path and a config
  location, unit-tested for: `.idea/` present, absent, present in a
  subdirectory, already-registered, malformed config — no test may touch
  the real `$HOME`
- [ ] **Given** Windows, **then** the warning either names the correct
  platform path or stays silent, and never prints a Unix path to a Windows
  user — `spawnWarning`'s precedent of skipping the platform it cannot
  advise on is an acceptable answer here
- [ ] The README notes that adoption detects this case and says so, so the
  docs and the tool agree about who is responsible for the JetBrains file
