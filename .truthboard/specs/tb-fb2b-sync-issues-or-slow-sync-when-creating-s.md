---
id: tb-fb2b
title: Sync issues or slow sync when creating stories or refreshing board sometimes
owner: emmanuel
branch: '*/tb-fb2b-*'
paths:
    - internal/web/**
priority: 2
type: bug
---

## Goal

Saving a story on a shared board takes seconds, and a story someone
else creates takes seconds more to appear. Neither is mysterious once
the path is traced, and only part of it is actually a bug.

**Saving blocks on two network round-trips.** `respondIntent` answers
only after `committer.land` has run `add`, `commit`, `pull --rebase`
and `push` against origin — so a save from a phone waits out a rebase
and a push to the forge before the dialog closes. That is deliberate:
the push error has to reach the person who made the edit, not just a
server log. But nothing tells them it is working, so it reads as a
hang. `land` also holds a mutex, so two people saving at once queue.

**A new story is announced to nobody.** The SSE broadcaster exists for
exactly this and `live.notify()` is called on the webhook path only
(`server.go:224`) — never after an intent write. So another viewer's
board updates on its own 3s poll rather than immediately, and the
channel built to carry the news sits idle.

**Every write invalidates the whole audit.** `cache.invalidate()` forces
the next `/api/board` to re-derive from scratch, walking every branch of
every spoke. On a hub with three spokes that is the pause after a save.

**A dialog freezes the board.** `app.js` skips rendering while a dialog
is open, so nothing lands while the editor is up. Correct — it must not
yank a form out from under someone — but it looks like sync stopped.

## Acceptance

- [ ] An intent write notifies the broadcaster, so other viewers see a
      new or edited story without waiting for their next poll
- [ ] Saving reports progress rather than appearing to hang; whatever
      shape that takes, a push failure still reaches the person who
      saved, since the spec is written either way and only the push can
      fail (see the contract in `respondIntent`)
- [ ] Two people saving at once do not serialize into a visible stall
- [ ] A cache invalidation does not re-derive spokes whose refs have not
      moved
- [ ] Measured, not assumed: record save-to-visible latency on a hub
      with remote spokes before and after, and put the numbers in the
      commit

## Notes

The dialog-render guard is working as intended — decide whether to
surface "there are changes waiting" rather than change the behaviour.
Perceived-speed work overlaps [[tb-c469]]; if that lands first, some of
this becomes presentation only.
