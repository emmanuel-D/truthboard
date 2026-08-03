---
id: tb-02da
title: Status stops claiming a current release it cannot keep up with
owner: emmanuel
branch: '*/tb-02da-*'
paths:
    - README.md
epic: ship-readiness
priority: 1
type: bug
---

## Goal

tb-e0b1 rewrote `## Status` and its last acceptance line was *"no version
number is written anywhere it will silently rot"*. That box was ticked
and the text rotted **within the hour**: the section says "`v0.11.0` is
the current release" and lists five stories as "shipping with the next
tag" — a tag cut minutes later. Same failure the story existed to fix,
one release later.

The lesson is that a hand-maintained "current release" line cannot be
kept correct by discipline, because the moment it is written the next tag
is already pending. It is exactly the class of claim Truthboard exists to
refuse: a status somebody types, drifting from the artefact that proves
it. The README should hold the same line the product does — describe what
the thing *is*, and let the tags say which version you get.

So: no version number, no "shipping with the next tag" promise. Point at
Releases, which is generated and cannot lie, and spend the section on
what a reader actually needs — what Truthboard does and how far along it
is. Nothing here should need editing when a tag is cut.

## Acceptance

- [x] `## Status` names no specific version and makes no claim about what is or is not yet released
- [x] It links to Releases for the published record
- [x] Cutting a tag requires no edit to this section — the release checklist has one less step, not one more
- [x] The section still tells a first-time reader what state the project is in
