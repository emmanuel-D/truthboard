---
id: tb-0274
title: 'Sprint planning summary: truthboard plan narrates the sprint you are about to start'
owner: emmanuel
branch: '*/tb-0274-*'
paths:
    - internal/audit/**
    - internal/llm/**
    - internal/report/**
    - cmd/truthboard/**
epic: po-experience
priority: 1
---

## Goal

`truthboard review <sprint>` narrates the sprint that just ended, but the
meeting at the *other* end of the boundary has no command. A team starting
s13 has to assemble by hand what the audit already knows: what rolls over
from the closing sprint, which candidates are ready versus blocked by
`needs:`, and how many points they are about to commit.

Add the symmetric half — `truthboard plan [sprint]` — built the same way
`review` is: a derived facts block that an LLM rephrases, never a source of
truth. Nothing new is typed. Every input already exists as intent (`sprint`,
`priority`, `points`, `needs`) or is derived (statuses, rollups).

The facts a plan needs:

- **Rollover** — the open stories of the active/most recently completed
  sprint, each with its derived status, so "planned" and "started but not
  landed" are distinguishable.
- **Candidates** — unsprinted, not-done stories in backlog order (priority,
  then id), split into ready and blocked, with the blocking spec ids named.
- **Load** — committed points for the target sprint against the closing
  sprint's `PointsDone`, plus the unestimated count so the sum is never
  read as complete when it isn't.
- **Window** — the target sprint's dates and derived state, when a sprint
  intent file exists.

`plan` with no slug plans the next future sprint if a dated one exists,
otherwise it reports candidates and rollover without a target.

The "does it fit" question stays honestly answerable only against one prior
sprint — there is still no velocity history, and this story does not invent
one. Say so in the facts rather than implying a trend.

## Acceptance

- [ ] `truthboard plan [sprint]` narrates a planning summary from derived facts, mirroring how `review` is wired (a `planFacts` beside `reviewFacts`, an LLM that only rephrases)
- [ ] A `PlanRollup` in `internal/audit` carries rollover, ready/blocked candidates in backlog order, committed points vs the closing sprint's landed points, and the unestimated count
- [ ] Blocked candidates name the spec ids from `needs:` that are not yet done
- [ ] `plan` with no sprint argument targets the next future dated sprint, or reports rollover and candidates with no target when none exists
- [ ] The rollup is on the audit `Result` and appears in `--format json`, so the summary is reproducible without an LLM key
- [ ] A repo with no sprints and no candidates fails loudly with a useful message rather than narrating an empty block
- [ ] No new intent field is introduced and no status is typed
