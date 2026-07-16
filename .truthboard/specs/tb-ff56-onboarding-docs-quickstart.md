---
id: tb-ff56
title: 'Onboarding docs: install + quick start in README and help'
owner: emmanuel
branch: '*/tb-ff56-*'
paths: [README.md, cmd/truthboard/**]
epic: ship-readiness
priority: 1
---

## Goal

A newcomer landing on the README, or typing `truthboard help`, should see
the path into an existing project in under ten seconds: get the binary,
`init`, open the board. Today both document features, neither documents
the journey.

## Acceptance

- [x] README opens with Install (release binaries + go install) and a
      Quick start for an existing project (init --agents --hooks, board,
      npm-scripts variant) before any feature section
- [x] `truthboard help` ends with a "getting started in a repo" block
      showing the same three-command path
- [x] Every command in help supports `-h` for its flags (asserting it
      found and fixed status/stop treating -h as a repo path)
