---
id: tb-8de2
title: Installing truthboard, you can see what you are getting
owner: emmanuel
branch: '*/tb-8de2-*'
epic: ship-readiness
priority: 1
type: story
---

## Goal

The README's two screenshots are from 2026-07-17 — three releases and a
redesign behind what installs today (v0.19.0). They show neither the panel
rhythm of v0.18.1, nor the responsive rows, nor the clickable story ids. A
newcomer deciding whether to install is being shown a product that no longer
exists. Worse, the demo repo they were taken from was never committed, so
refreshing them means rebuilding a repo by hand — which is exactly why they
went stale.

Two things follow. Capture what ships now, from a demo repo that lives in the
tree, so the next refresh is a command rather than an archaeology exercise.
And give the person who just ran the install script the walkthrough this
README does not have: what the first commands actually print, what the board
shows them, and where to go next. At 850 lines organised by feature
philosophy, the page serves the reader who already understands the tool and
abandons the one who does not.

Scope: README.md, docs/**.

## Acceptance

- [ ] A committed script rebuilds the demo repo from nothing, so screenshots can be refreshed without inventing a repo by hand
- [ ] README screenshots are captured from the version that installs today, and show the surfaces it actually ships
- [ ] Captions say what each picture proves, not merely what it contains
- [ ] A first-run walkthrough shows the real terminal output of init and audit — what the reader will actually see
- [ ] The reader can find a section: the README opens with a map of itself
- [ ] `go test ./...` passes
