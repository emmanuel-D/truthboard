---
id: tb-ee45
title: 'init speaks npm: board scripts in package.json'
owner: emmanuel
branch: '*/tb-ee45-*'
paths: [internal/adopt/**, cmd/truthboard/**]
epic: po-experience
priority: 1
---

## Goal

A JS team shouldn't have to learn a new CLI to keep their board around.
When `truthboard init` runs in a repo with a `package.json`, it wires the
lifecycle into the ecosystem the team already uses: `npm run board`,
`npm run board:status`, `npm run board:stop`, `npm run board:audit`.
Delegated to `npm pkg set` so npm itself preserves the file's structure —
we never hand-mangle someone's package.json.

## Acceptance

- [ ] `truthboard init` in a repo with package.json adds the four board
      scripts; `npm run board` starts the detached board
- [ ] Existing scripts with the same name are never overwritten — skipped
      and reported
- [ ] Running init twice reports "already there", changes nothing
- [ ] No package.json, or no npm on PATH: skipped with a one-line note,
      never an error
