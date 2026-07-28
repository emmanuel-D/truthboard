---
id: tb-1b2e
title: truthboard mcp serves the directory you point it at
owner: emmanuel
branch: '*/tb-1b2e-*'
paths:
    - cmd/truthboard/main.go
    - internal/mcp/**
epic: mcp-server
priority: 2
type: bug
---

## Goal

`truthboard mcp` hardcodes the repository to the process working
directory — `cmd/truthboard/main.go:140` calls
`mcp.Serve(os.Stdin, os.Stdout, ".", version)`. A path passed on the
command line is accepted without complaint and then ignored, so
`truthboard mcp /path/to/hub` silently serves whatever directory the
client happened to spawn the process in.

Every other subcommand takes an optional `[repo]` — `audit`, `next`,
`board`, `ui`, `init` all do. `mcp` is the outlier, and it is the one
command whose working directory the user least controls, because the
MCP client chooses it.

This bites multi-repo hubs. When `.truthboard/` lives in a subdirectory
(the documented hub-as-subdir layout from `init --workspace`), an agent
started in the parent gets a server that answers every tool call with
"not a git repository". The only workaround today is a shell wrapper in
`.mcp.json`:

    "command": "sh",
    "args": ["-c", "cd /abs/path/to/hub && exec truthboard mcp"]

which bakes a machine-specific absolute path into a file that is
otherwise portable and committed.

Found while wiring the LetTalk hub so agents could be launched from the
workspace parent rather than the hub subdirectory.

## Acceptance

- [ ] `truthboard mcp [repo]` accepts an optional repo path and serves that repository
- [ ] With no argument the behaviour is unchanged: the current directory is served
- [ ] A path that is not a git repository fails at startup with the same git-doctor message the other commands emit, rather than starting and failing per tool call
- [ ] The argument appears in `truthboard mcp --help` and in the top-level usage listing, matching the `[repo]` convention of the other subcommands
- [ ] A test serves a repository other than the process working directory and asserts a tool call reads specs from it
- [ ] `init`/`adopt`-generated `.mcp.json` stays portable — no absolute paths introduced
