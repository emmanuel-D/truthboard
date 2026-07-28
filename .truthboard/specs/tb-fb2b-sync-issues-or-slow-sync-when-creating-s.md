---
id: tb-fb2b
title: Sync issues or slow sync when creating stories or refreshing board sometimes
owner: emmanuel
branch: '*/tb-fb2b-*'
paths:
    - internal/web/**
    - internal/audit/**
priority: 2
type: bug
---

## Goal

Saving a story on a shared board takes seconds, and the board itself
intermittently refuses to refresh. Measurement found three causes, and
only one of them was the one originally guessed.

**Linking asked git a question per (spec × branch) pair.** `linkSpecs`
ran `git log --grep <trailer>` once for every spec against every branch,
so this repo — 68 specs, 17 refs — spawned 940 git processes for one
audit and took fifteen seconds. `boardCache.get()` holds its mutex for
the whole audit while browsers poll every three seconds, which is
exactly the "audit unavailable — retrying" the board shows. Specs are
intent and cost nothing to write, so their number must never multiply
the work.

**Saving paid a forge round-trip for a rebase that did nothing.**
`committer.land` rebased on origin before every push. The board is
usually the only writer and the sync loop has already fetched, so that
first round-trip almost always changed nothing — and someone waiting
for a dialog to close paid it in real seconds.

**A new story was announced to nobody.** `live.notify()` fired on the
webhook path only, never after an intent write, so another viewer's
board waited out its own 3s poll. The one change most worth announcing
was the only one not using the channel built to announce it.

Two things that look like bugs and are not: `land` holds a mutex, but
git on one working tree is serial and always will be — the guarantee is
that every save lands, not that saves overlap. And `app.js` refuses to
re-render under an open dialog, which is correct; it must not yank a
form out from under someone.

## Acceptance

- [ ] Adding specs to an unchanged repo adds no git processes to an
      audit, guarded by a test that fails on the per-pair form
- [ ] The derivation is unchanged: same statuses, same landings, same
      evidence, for every spec and unit
- [ ] An uncontended save costs one round-trip to the forge, not two,
      and a push refused for a reason a rebase cannot fix surfaces as
      itself rather than being retried
- [ ] A save whose push is rejected as stale still rebases and arrives
- [ ] An intent write notifies the live channel, so other viewers see a
      new story without waiting for their next poll
- [ ] Saving reports progress rather than appearing to hang, and a push
      failure still reaches the person who saved
- [ ] Measured, not assumed: round-trip counts and audit timings in the
      commit

## Notes

Perceived-speed work overlaps [[tb-c469]]. The dialog-render guard is
worth surfacing as "there are changes waiting" rather than changing.
