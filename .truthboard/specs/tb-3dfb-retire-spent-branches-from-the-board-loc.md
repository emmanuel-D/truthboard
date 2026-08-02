---
id: tb-3dfb
title: Retire spent branches from the board, local and on origin
owner: emmanuel
branch: '*/tb-3dfb-*'
paths:
    - internal/web/**
    - internal/audit/**
    - internal/gitrepo/**
epic: po-experience
priority: 1
---

## Goal

The board already knows which branches are spent: every unit derived `done`
is a branch whose commits are in the integration branch, and the drift
report has carried them as "landed, not deleted" since the first version —
a finding with nothing to do about it. Acting on it means leaving the
board, listing branches by hand, and remembering which ones were merged.

Give that derived truth a button. One branch at a time, local ref and
origin ref, behind a deliberate two-step confirmation — and refuse, by
default, anything git says still carries unmerged work.

Deleting a branch mutates proof, not intent, which is new for this server:
it is gated exactly like an intent write (loopback, or the edit token on a
shared board), it never touches an integration branch, and it says out loud
what it did and what it could not do.

## Acceptance

- [ ] The Branches panel lists each branch with a delete button and says which refs exist — local, origin, or both
- [ ] Deleting asks twice in a modal that names the branch and the exact refs that will go
- [ ] A branch whose work is not in the integration branch is refused, naming what would be lost; an explicit override deletes it anyway
- [ ] The integration branch and the checked-out branch can never be deleted, whatever the request says
- [ ] Deleting the origin ref pushes the deletion; a push that fails is reported in the UI, not only in a log
- [ ] A read-only shared board offers no deletion at all, and a token-armed board requires the token
- [ ] Tests cover merged deletion of both refs, the unmerged refusal and its override, the integration-branch refusal, and the missing-token refusal
