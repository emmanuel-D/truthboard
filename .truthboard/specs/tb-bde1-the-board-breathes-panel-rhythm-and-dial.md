---
id: tb-bde1
title: 'The board breathes: panel rhythm, and dialogs that use a desktop screen'
owner: emmanuel
branch: '*/tb-bde1-*'
paths:
    - internal/web/static/**
epic: po-experience
priority: 1
type: bug
---

## Goal

Two things the board gets wrong once it is more than a few panels long,
both visible the moment it is opened on a laptop rather than a phone.

The panels below the kanban — summary, plan, sprints, flow, drift,
branches — are emitted one after another into `#app` with no vertical
rhythm at all. `* { margin: 0 }` resets everything and only `.grid2`
carries a `margin-bottom`, so the standalone panels sit edge to edge:
two hairline borders touching, no gap, and the page reads as one
undifferentiated wall instead of a stack of separate answers.

The dialogs are sized for the phone they were fixed on. `min(38rem, 94vw)`
is a column 600px wide on a 1600px screen: the story detail wraps its
prose and stacks two short truth blocks under each other with two thirds
of the screen empty beside them, and the editor makes the author choose
between writing and previewing on a display with room for both.

## Acceptance

- [x] Every top-level block in `#app` is separated by one consistent gap — no two panel borders touch
- [x] The gap comes from one rule, not a per-panel margin, so a new panel inherits the rhythm without being told
- [x] The story detail dialog widens on a desktop screen and its two truth blocks sit side by side rather than stacked
- [x] Rendered story prose keeps a readable measure however wide the dialog gets
- [x] The editor dialog widens on a desktop screen and its field grid uses the extra width instead of stretching two fields
- [x] On a wide screen the editor shows Write and Preview at once, live, and the tab buttons that only existed to choose between them go away
- [x] Below that width nothing changes: the tabs come back and one pane shows at a time
- [x] Resizing across the breakpoint with the editor open lands in the right layout either way
- [x] Phone layout is unchanged — dialogs still fit 390px and the deck still prints
