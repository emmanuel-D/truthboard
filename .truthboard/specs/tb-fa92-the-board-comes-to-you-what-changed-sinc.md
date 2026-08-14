---
id: tb-fa92
title: 'The board comes to you: what changed since, delivered on a schedule'
owner: emmanuel
branch: '*/tb-fa92-*'
paths:
    - internal/audit/**
    - internal/report/**
    - internal/web/**
    - cmd/truthboard/**
epic: po-experience
priority: 2
type: story
---

## Goal

Everything this tool knows waits for someone to come and ask. The one
exception is `ui --notify`, which posts stalled and regressed
transitions — it proves the plumbing works, and it is the only thing
that ever leaves the process.

Two everyday moments go unserved as a result. **Standup**: "what moved
since yesterday?" has no answer short of reading the whole board and
remembering what it said the last time you read it. **Morning**: nobody
learns that a story stalled, a merge regressed one, or a landed story is
carrying unticked acceptance, unless they happen to look — and the
people who most need to know are the least likely to open a terminal.

The facts already exist and are already derived. What is missing is a
*difference between two points in time*, and a way to have it arrive.
No API key should be involved: this is the plain-facts path that
`summary` already walks, not a narration.

## Acceptance

- [ ] `truthboard since <ref|date>` reports what changed on the board between that point and now — stories that entered or left each derived status, acceptance ticked, drift opened and closed — and says plainly when nothing changed
- [ ] The comparison is derived from git, needs no stored snapshot or state file, and gives the same answer whoever runs it and whenever
- [ ] The same difference can be posted to the existing `--notify` webhook on an interval, alongside the transitions that already go there
- [ ] A scheduled digest includes landed stories still carrying unticked acceptance criteria, naming them and the counts
- [ ] Repeated runs do not repost what was already sent, and a run that finds nothing worth reporting stays silent rather than posting an empty digest
- [ ] It works without `ANTHROPIC_API_KEY` or `OLLAMA_HOST` — derived facts, plain language, no narration
- [ ] Credentials in a webhook URL never reach the logs, holding the line `tb-5266` drew
- [ ] `go test ./...` passes
