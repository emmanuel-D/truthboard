---
id: tb-eeb7
title: 'Why work is paused: a hold reason that git can contradict'
owner: emmanuel
branch: '*/tb-eeb7-*'
paths:
    - internal/spec/**
    - internal/audit/**
    - internal/mcp/**
    - internal/web/**
epic: po-experience
priority: 1
---

## Goal

Git proves that work *stopped*. It cannot prove *why*. The board can say
"no commits for 13 days" and "waiting on tb-5c6d", but the reason a PO
actually needs — deprioritised, waiting on legal, the vendor hasn't
replied, the person is on leave — lives in someone's head and no amount
of history will produce it.

Add `hold:` to a spec's frontmatter: one line of plain human intent.

    hold: waiting on legal sign-off for EU tax rates

This is intent, not a status. It is written by hand like `title` or
`epic`, it never affects a derived status, and there is still nothing to
set — a story does not become "on hold", it *is* stalled or planned by
git's account and separately carries a human note explaining it.

The obvious objection is that a hand-maintained note is exactly the kind
of field Truthboard exists to abolish: someone writes "waiting on legal",
legal replies, work resumes, and the note lies forever. So the note is
not trusted blindly — git gets to contradict it. A hold on a story that
git shows as *actively progressing* (recent commits) or *already done* is
contradicted by the evidence, and every surface says so rather than
repeating it. The note is intent; whether it still holds is derived.

That keeps the contract intact: a hold reason can be wrong, but it cannot
be wrong *silently*.

## Acceptance

- [ ] `hold:` is read from spec frontmatter, carried on `SpecStatus`, and writable through `update_spec` over MCP, the CLI, and the web intent editor — like any other intent field
- [ ] A hold note never changes a derived status; a story with a hold is still exactly as planned/stalled/in-progress/done as git says it is
- [ ] A hold on a story deriving as done, or one with commits inside the stale window, is reported as contradicted — the note and the contradiction travel together, never the note alone
- [ ] Contradicted holds appear in the drift report, next to stale promises and shadow work
- [ ] The board, brief, and `--format json` all carry the hold and its contradicted flag
- [ ] Clearing a hold is deleting the line — there is no "unhold" verb
- [ ] A repo that never writes a hold sees nothing new anywhere
