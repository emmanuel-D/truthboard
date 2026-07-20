---
id: tb-0ac4
title: The webhook test races its own background refresh
owner: emmanuel
branch: '*/tb-0ac4-*'
paths: [internal/web/**]
epic: ship-readiness
priority: 1
---

## Goal

`TestWebhookRequiresTheSecret` fails intermittently in CI:

    --- FAIL: TestWebhookRequiresTheSecret
        TempDir RemoveAll cleanup: unlinkat …/001/.git: directory not empty

Caught on 2026-07-20 during the tb-c4c3 work — the same commit had passed
twenty minutes earlier with no Go change between the runs, which is what
identifies it as a race rather than a defect in the assertions.

The test posts a webhook with the correct secret, which is the whole point
of the case. That kick starts a refresh on a background goroutine, and the
goroutine runs git inside the fixture repo. Nothing awaits it: the test
returns, `srv.Close()` runs, and `t.TempDir()` starts deleting the
directory while git is still creating files under `.git`. Whether it fails
depends on which finishes first.

Fixing this by sleeping would trade a flake for a slow test that still
races on a loaded runner. The refresh needs to be awaitable — a signal the
server can expose for tests (and, plausibly, for the webhook response
itself, which currently promises a refresh it does not wait for).

This matters more here than in most projects: truthboard derives
`regressed` from red CI, so a test that fails at random makes the board
report a regression that never happened. A tool whose claim is that its
findings are trustworthy cannot have a random red in its own pipeline.

## Acceptance

- [ ] The webhook tests pass repeatedly under load — `go test -race -count
      20 ./internal/web/` is green
- [ ] No test sleeps to avoid the race; the refresh is awaited on a real
      signal
- [ ] A webhook request does not report success before the refresh it
      triggered has been applied, or the response says plainly that the
      refresh is asynchronous
- [ ] The fixture repo is never written to after the test that owns it has
      returned
