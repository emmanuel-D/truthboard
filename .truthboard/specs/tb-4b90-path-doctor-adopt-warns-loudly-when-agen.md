---
id: tb-4b90
title: 'PATH doctor: adopt warns loudly when agents cannot spawn truthboard'
owner: emmanuel
branch: '*/tb-4b90-*'
paths: [internal/adopt/**]
epic: agent-loop
priority: 1
---

## Goal

`.mcp.json` registers the server as bare `command: "truthboard"`, resolved
against the PATH of whatever launched the agent — not the user's shell.
In the retropixel3d-mono pilot the binary lived in `~/go/bin` (on PATH only
via `.zshrc`), so Claude Code's MCP spawn died in 2ms with "Executable not
found in $PATH" and the agent never knew the board existed. Adopt promised
an integration it couldn't deliver, silently.

Fix: after registering MCP, adopt checks whether a `truthboard` executable
is reachable from the standard system locations GUI-launched agents
actually get (`/usr/local/bin`, `/opt/homebrew/bin`, `/usr/bin`, `/bin`).
When it isn't, print a loud warning naming where the running binary lives
and the exact symlink command that fixes it. The registration stays a bare
command name — `.mcp.json` is committed and shared, so no machine-specific
absolute paths in it.

## Acceptance

- [ ] Adopt with the binary only in a shell-profile dir (e.g. `~/go/bin`)
      prints a warning naming the binary's real location and a copy-paste
      `ln -s` fix into a system dir
- [ ] Adopt with truthboard reachable in a system dir stays quiet — no
      warning, action log unchanged
- [ ] The warning never fails the adoption: exit code stays 0, all other
      steps still run
- [ ] `.mcp.json` still registers the bare `truthboard` command — never an
      absolute path
