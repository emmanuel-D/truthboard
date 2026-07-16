---
id: tb-88bc
title: CI on the truthboard repo
owner: emmanuel
branch: '*/tb-88bc-*'
paths: ['.github/**']
epic: ship-readiness
priority: 1
---

## Goal

The tool that flips specs to regressed on red CI has no CI of its own. Every
push and PR must prove the inference engine still holds before it lands —
and give our own `regressed` detection a real signal to read on this repo.

## Acceptance

- [x] A workflow runs `go vet` and `go test ./...` on every push to main and every PR
- [ ] gofmt violations fail the build (diff shown in the log)
- [x] The workflow is green on main after merge
