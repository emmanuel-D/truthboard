---
id: tb-f82e
title: Board dialogs scroll properly and the serving version is always visible
owner: emmanuel
branch: '*/tb-f82e-*'
paths:
    - internal/web/**
epic: po-experience
priority: 1
type: bug
---

## Goal

Two reported UI defects. (1) Modals don't scroll properly: .dbody caps
its height with a hardcoded calc(86vh - 8rem) guess at header/footer
size, but the detail dialog's chips and assign rows — and the editor's
grown field grid — exceed it, so content overflows the dialog's own
max-height and the footer buttons clip out of reach. Replace the magic
number with a flex-column dialog whose body flexes and scrolls. (2) The
serving version only appears in the footer after the first successful
poll and reads just "dev" for source builds; surface the version
reliably (header meta too) and label dev builds honestly.

## Acceptance

- [ ] A story with a long body opens with its footer buttons reachable and the body scrolling, on a short viewport
- [ ] The editor on a short viewport keeps Save/Cancel reachable while the form scrolls
- [ ] No hardcoded chrome-height calc remains in dialog CSS
- [ ] The serving version is visible in the page chrome (release tag, or an explicit source-build label)
