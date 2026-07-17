---
id: tb-c599
title: 'Multi-machine board: --fetch keeps a remote-watching board fresh'
owner: emmanuel
branch: '*/tb-c599-*'
paths:
    - internal/web/**
    - internal/lifecycle/**
    - cmd/truthboard/**
    - README.md
epic: po-experience
priority: 1
---

## Goal

Today the board is only as fresh as the local clone: it derives everything
from local refs and the working tree, so on a PO's machine (or a shared
box) it silently drifts until someone remembers to `git pull`. That breaks
the core promise — the board must reflect repo reality, and reality lives
on the remote.

Add an opt-in sync loop to `truthboard ui`:

- `--fetch <interval>` (e.g. `--fetch 60s`, off by default) runs
  `git fetch --prune origin` in the background so remote-tracking refs —
  which the audit already reads — stay fresh with zero local git action.
- Spec files are intent and live in the working tree, so after a fetch the
  loop fast-forwards the checkout **only** when it is clean and on the
  integration branch; otherwise it skips the pull and says so. It must
  never touch dirty work or a feature-branch checkout.
- The page shows sync freshness (last sync, or the error when fetching
  fails) so a stale board is never mistaken for a quiet repo.
- `--host` lets a shared box serve the board beyond loopback; when the
  listener is not loopback, intent writes are disabled (read-only board)
  because there is no auth story yet — the guard says exactly that.
- `--detach` propagates `--fetch` and `--host` so the usual daily setup
  (`truthboard ui --detach --fetch 60s`) just works.

## Acceptance

- [ ] `truthboard ui --fetch 60s` fetches origin on the interval; a branch
      pushed from another machine changes the board with no local git use
- [ ] the checkout is fast-forwarded only when clean and on the integration
      branch; dirty tree or feature-branch checkout skips the pull, and the
      board surfaces which (fresh refs, stale intent)
- [ ] the page shows when the remote was last synced and shows fetch
      failures loudly instead of pretending freshness
- [ ] with `--host` beyond loopback the board serves read-only: spec
      create/edit routes refuse with a clear message, and the UI hides
      editing affordances
- [ ] `--detach` carries --fetch/--host into the background process and
      `truthboard status` reports them
- [ ] README documents the multi-machine setup (PO laptop and shared box)
