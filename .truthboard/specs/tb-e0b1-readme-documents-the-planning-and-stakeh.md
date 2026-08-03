---
id: tb-e0b1
title: README documents the planning and stakeholder release before it is tagged
owner: emmanuel
branch: '*/tb-e0b1-*'
paths:
    - README.md
epic: ship-readiness
priority: 1
---

## Goal

Five stories landed since v0.11.0 and the README describes none of them.
Worse, `## Status` still announces `v0.6.0` and says multi-repo "ships
with the next tag" — that tag was v0.7.0, in July. A release cut now
would publish a front page that is five releases behind and silent about
the features the release exists for.

Three of the five are user-facing vocabulary or commands a reader cannot
discover any other way:

- `truthboard summary [sprint] [--ids]` — what was delivered and for how
  many points, what is paused and why, in plain language with no API key.
  Undocumented.
- `hold:` — the one intent field a human writes that git cannot produce,
  and the contradiction rule that keeps it from rotting. Undocumented,
  and it belongs beside `sprint`/`points`/`needs` in the intent
  vocabulary where someone will actually meet it.
- `truthboard plan` — documented, but only in the LLM section. Its data
  is on the board and in `--format json` with no key at all, which is
  the more useful half and is currently buried.

The board's two new panels (the sprint about to start, and the plain
-language summary) also go unmentioned in the web-board section.

Keep `## Status` honest about the split it has always described: what is
released, versus what is on `main` awaiting a tag. That distinction is
the reason the section exists and is exactly what went stale.

## Acceptance

- [x] `## Status` names the current released tag and says plainly what sits on `main` unreleased, with no leftover claim about work that shipped months ago
- [x] `truthboard summary` is documented with its scope argument, `--ids`, and the fact that it needs no API key or network
- [x] `hold:` appears in the intent vocabulary beside the other frontmatter fields, including that git contradicts a note the evidence disagrees with
- [x] `truthboard plan`'s no-key path (the `plan` object in `--format json`) is stated where a reader looking for planning will find it
- [x] The web-board section mentions the planning and summary panels
- [x] No version number is written anywhere it will silently rot — prefer naming the release the text is about over hardcoding a "latest"
