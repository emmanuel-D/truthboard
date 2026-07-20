---
id: tb-d146
title: Re-running init upgrades an existing commit-msg hook
owner: emmanuel
branch: '*/tb-d146-*'
paths: [internal/adopt/**]
epic: agent-loop
priority: 1
---

## Goal

`installHook` returns `already installed` the moment it finds `hookMark` in
`.git/hooks/commit-msg` (`adopt.go:317`) and never touches the text again.
The hook is frozen at whatever version first wrote it — permanently, even
on an explicit `truthboard init --agents --hooks`. That breaks the promise
in the package's own doc comment, which `AGENTS.md` and `CLAUDE.md` keep
via `upsertBlock`: marker-delimited blocks are replaced, never duplicated.

Found immediately downstream of [[tb-3d43]], which taught the nudge to stay
quiet on intent-only commits. On this repo the hook kept warning after
v0.8.2 was installed, because the file on disk was written 2026-07-16.
**The consequence is that the tb-3d43 improvement cannot reach a single
existing adopter** — the only remedy today is to delete the hook by hand
and re-run, which nobody will know to do. Any future nudge change inherits
the same fate.

The migration is the real design work, and the reason this is a story
rather than a one-line change. `hookMark` is an opening marker with **no
closing counterpart**, so the block cannot simply be bounded and swapped
the way `upsertBlock` does — and the nudge may sit inside a hook someone
else owns, where over-reaching would destroy their script. Adding an end
marker fixes the future but leaves every already-installed hook
unbounded, so upgrading those needs its own decision: recognise the known
historical nudge texts and replace exactly those, or leave anything
unrecognised alone and say loudly that the hook is outdated and how to
refresh it. Either is acceptable; silently rewriting a hook truthboard
cannot prove it authored is not.

## Acceptance

- [ ] Re-running `init --agents --hooks` on a repo whose nudge is outdated
      leaves the current nudge text in `.git/hooks/commit-msg`
- [ ] A nudge that is already current is a logged no-op — file mtime and
      contents unchanged
- [ ] Lines a third party added to the hook, before or after the nudge,
      survive the upgrade byte-for-byte
- [ ] A hook carrying a nudge truthboard cannot recognise is never
      rewritten silently: it is either upgraded correctly or reported as
      outdated with the manual fix named
- [ ] The nudge stays exit-code-neutral after upgrade — it warns, never
      blocks, whatever the surrounding hook decides
