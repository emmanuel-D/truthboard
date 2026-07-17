---
id: tb-40c4
title: Split the web page into embedded static assets
owner: emmanuel
branch: '*/tb-40c4-*'
paths:
    - internal/web/**
epic: po-experience
priority: 1
type: task
---

## Goal

The board is one ~900-line index.html mixing markup, CSS, and JS in
template-literal soup — a previous session broke the whole page with one
bad edit into its if/else chains, and bugs like clipped modals slip
through because nothing lints or diffs cleanly. Split it into
internal/web/static/ (index.html + app.css + app.js) embedded as a
directory. Still zero build step, still one binary — just organized,
diffable, and syntax-checkable files.

## Acceptance

- [ ] Page is served from an embedded static/ dir: index.html, app.css, app.js as separate files
- [ ] go build remains the entire pipeline — no node/build step introduced
- [ ] The board renders identically before and after (verified in a browser)
- [ ] app.js passes a plain syntax check as a standalone file
