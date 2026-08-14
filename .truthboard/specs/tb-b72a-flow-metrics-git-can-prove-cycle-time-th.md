---
id: tb-b72a
title: 'Flow metrics git can prove: cycle time, throughput, work in progress'
owner: emmanuel
branch: '*/tb-b72a-*'
paths:
    - internal/audit/**
    - internal/report/**
    - internal/tui/**
    - internal/web/**
    - cmd/truthboard/**
epic: po-experience
priority: 1
type: story
---

## Goal

Every status this tool serves is "now". A story is planned, in progress,
done — and the moment it flips, how long it took stops being knowable
from the board. Ask "how long does a story actually take here?" and the
answer is nowhere, even though the repo holds every timestamp needed to
compute it: the first commit carrying a `Spec: <id>` trailer, and the
merge that put it on the integration branch.

This is the thesis applied to flow. A tracker asks people to record
cycle time, so cycle time is fiction — it measures who remembered to
drag the card. Here it is *derived*, from the same evidence that already
derives status: unfakeable, free, and true for work nobody logged.

The gap is explicit in the code. `internal/audit/plan.go:18` declines a
velocity trend — "one prior sprint is one data point" — which is the
right call given what `plan` can see today, and the wrong outcome: the
data is in git, it has simply never been mined.

Honesty constraints, same as everywhere else: a story whose evidence is
too thin to time (no trailer before the merge, a squash that erased the
history, work that predates adoption) must say so rather than produce a
confident number, and nothing here may feed back into a derived status.

## Acceptance

- [ ] Per-story cycle time is derived from git alone — first commit carrying the trailer to the merge onto the integration branch — with no field to type and no field to maintain
- [ ] A story whose history cannot support a measurement is reported as unmeasurable, naming why, and is excluded from aggregates rather than counted as zero or guessed
- [ ] Throughput (stories and points landed per week and per sprint) and work in progress over time are derived from the same evidence
- [ ] The measurements appear in `truthboard audit`, `audit --format json`, the markdown report, the TUI and the web board, consistently and from one code path
- [ ] `truthboard plan` and `truthboard review` narrate real history where it exists, and keep declining to invent a trend where it does not
- [ ] Aggregates state the window they cover and the number of stories behind them, so a figure from three stories cannot read like a figure from thirty
- [ ] No status is set, gated or downgraded by any of this; metrics observe the board, they never move it
- [ ] `go test ./...` passes
