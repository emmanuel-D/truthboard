---
id: tb-e8e3
title: Repository(ies) selection picks from the workspace, not free text
owner: emmanuel
branch: '*/tb-e8e3-*'
priority: 2
---

## Goal

When creating or updating a story, the `Repos` field must offer the
repos that exist rather than accept anything typed. A typo produced a
story that could never be done — done with `repos:` declared means the
trailer landed in every one of them, and a repo that does not exist
never lands anything.

Shipped first as a `<select multiple>`, which was the wrong control and
measurably so: with `[hub, server]` selected, one unmodified click on a
third option leaves `[connect]` alone. The safe gesture is ⌘-click, the
default gesture is destructive, and on a phone — where this board is
meant to be used — there is no ⌘-click at all. For a field that decides
what *done* requires, silently dropping two repos is not a papercut.

Toggle chips instead, reusing the `.fchip` idiom the filter bar already
uses for the very same concept, so repos look the same in both places.

## Acceptance

- [ ] The field offers the workspace's repos; nothing can be typed
- [ ] Each repo toggles independently — no gesture deselects the others
- [ ] Reuses the filter bar's chip idiom rather than adding a third
      visual language for repos on one page
- [ ] Keyboard reachable, and its pressed state exposed to assistive tech
- [ ] A repo the story declares but the manifest no longer has stays
      visible and deselectable, marked as unknown — dropping it silently
      would rewrite intent the board reports as drift
- [ ] A single-repo board hides the field, unless a stale declaration is
      there to clear
- [ ] Verified in a browser, including that a click on one chip leaves
      the others alone
