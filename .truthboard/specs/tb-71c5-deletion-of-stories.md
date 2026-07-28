---
id: tb-71c5
title: Deletion of Stories
owner: emmanuel
branch: '*/tb-71c5-*'
paths:
    - internal/web/**
    - internal/mcp/**
    - internal/spec/**
priority: 2
---

## Goal

A story created by mistake cannot be removed from the board. There is
no delete route at all: the write guard admits `POST /api/specs` and
`PUT /api/specs/` and nothing else (`server.go`), so the only way out
is to delete the file from a clone and push — which is exactly what had
to be done to clear a probe story off the live lettalk board.

Deleting a story is deleting its intent file, and that is an ordinary
commit. `committer.land` already handles it unchanged: `git add` on a
removed path stages the deletion, so the existing commit → rebase →
push path needs no new machinery. What needs deciding is what deletion
*means* when proof already exists.

**A story with proof must not vanish quietly.** Branches and trailers
carrying its id are facts in git; deleting the file erases the promise
while leaving the evidence, and the board would then show work nobody
can explain. A story nothing references is a different case — that is
the typo the operator wants gone.

**History is the undo.** A deletion is a commit, so recovery is a
revert. That is worth saying out loud rather than building an
"archived" state, which would be a status by another name — and
statuses are derived, never typed.

## Acceptance

- [ ] `DELETE /api/specs/<id>` removes the intent file, admitted by the
      write guard alongside the existing create and update routes
- [ ] On a token-armed shared board the deletion commits and pushes like
      any other intent write, and the commit carries no `Spec:` trailer
      (a trailer would derive the story it just deleted as done)
- [ ] Deleting a story that has proof — a branch matching its glob, or a
      commit carrying its trailer — is refused, and the refusal names
      what still references it
- [ ] The refusal can be overridden deliberately, so a story really can
      be retired without hand-editing a clone
- [ ] The UI offers deletion from the story detail view, confirms first,
      and says the recovery is `git revert`
- [ ] `delete_spec` over MCP, so an agent can retire a story it created
      by mistake — same guard, same refusal
- [ ] Covered by tests: a clean delete, a refused delete with proof, and
      a deletion landing on origin
