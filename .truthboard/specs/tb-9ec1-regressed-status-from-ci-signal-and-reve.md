---
id: tb-9ec1
title: Regressed status from CI signal and revert detection
owner: emmanuel
branch: '*/tb-9ec1-*'
paths: [internal/audit/**]
---

## Goal

Complete the V1 status model: a spec that was `done` must loudly become
`regressed` when reality moves backwards — its merge was reverted, or CI on
the integration branch went red on the commits that landed it. A silently
rotten `done` is exactly the kind of lie Truthboard exists to catch.

## Acceptance

- [x] A `git revert` of a spec-linked merge flips the spec from done to regressed
- [x] With `gh` available, red CI on the landing commit flips the spec to regressed
- [x] Regressed is rendered loudly (top of the board, red) in term/md/json
- [x] Without CI data the tool says nothing rather than guessing (honesty rule)
