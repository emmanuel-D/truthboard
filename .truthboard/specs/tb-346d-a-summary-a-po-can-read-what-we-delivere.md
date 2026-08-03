---
id: tb-346d
title: 'A summary a PO can read: what we delivered, what is paused and why'
owner: emmanuel
branch: '*/tb-346d-*'
paths:
    - internal/report/**
    - internal/audit/**
    - internal/web/**
    - cmd/truthboard/**
epic: po-experience
priority: 1
needs:
    - tb-eeb7
---

## Goal

Everything Truthboard renders today is written for the person who wrote
the commits. "Stale promises", "shadow work", "unmerged", branch names
before outcomes, `tb-2c3d` before "Invoice PDF export". The digest is
described as readable by a non-developer and only covers what *landed* —
it is silent on what is stuck, which is the half a PO actually asks
about.

Produce a summary for the person who does not read git: what we
delivered and for how many points, what is moving, what is paused and
for what reason, what has not started. Plain sentences, outcomes first,
no jargon and no identifiers unless asked for.

The vocabulary is the substance of this story, not decoration:

| derived | said as |
|---|---|
| done | delivered |
| in-progress | being worked on |
| stalled | paused |
| planned | not started yet |
| regressed | broke after delivery |

Reasons come from three places, in this order of confidence: a human
`hold:` note (tb-eeb7), an unmet dependency named by the *title* of the
blocking story rather than its id, and — only when neither exists — the
plain fact that nothing has been committed for N days. A contradicted
hold is never repeated as though it were current.

No LLM. `truthboard review` narrates beautifully and needs an API key; a
PO should not need one to learn what shipped. Every number here is
arithmetic the audit already did, so the summary is the same whether it
is generated on a laptop or a build server.

Two surfaces, one summary: a command that writes markdown someone can
paste into an email, and a panel on the board for the people who just
open the URL. They must not drift — same builder, two renderers.

## Acceptance

- [ ] `truthboard summary [sprint]` writes plain-language markdown covering delivered (with points), in flight, paused with reasons, and not started
- [ ] The default output contains no spec ids, no branch names and no derived-status words from the table above; `--ids` adds identifiers back for anyone who wants to look something up
- [ ] Points are stated as achievement ("13 of 21 points delivered"), with unestimated stories named as such rather than counted as zero
- [ ] Every paused story carries a reason: a hold note, or the title of the story blocking it, or how long it has been untouched — in that order of preference
- [ ] A contradicted hold (tb-eeb7) is never presented as a live reason
- [ ] The web board renders the same summary as a panel, built from the same function as the CLI, with a test asserting the two agree
- [ ] The summary needs no API key and no network
- [ ] With no sprint argument it summarises the digest window, mirroring how `review` behaves
