---
id: tb-8222
title: The README leads with the install path it can actually keep updated
branch: '*/tb-8222-*'
paths:
    - README.md
epic: ship-readiness
priority: 1
---

## Goal

The Install section offers three ways in and then one way to stay current —
"Stay current with `truthboard update`" — as if the update command were
independent of how you installed. It is not, and for one of the documented
paths the advice is actively wrong.

`selfupdate.replaceExecutable` resolves symlinks before swapping the binary,
so on a Homebrew install it writes straight into the Cellar. Brew's metadata
still reports the old version, and the next `brew upgrade` or `reinstall`
silently reverts the update. A reader who follows the README top to bottom —
`brew install`, then `truthboard update` — ends up in exactly that state.

Recommendation should be stated, not implied by ordering, and each install
path should carry the update command that belongs to it.

## Acceptance

- [x] The install script is named as the recommended path in words, not left
      to ordering alone, and stays first.
- [x] The reason is one line a reader can check: right build for the
      platform, checksum-verified, no sudo, and it is the path
      `truthboard update` keeps current.
- [x] Homebrew keeps its place as a genuine alternative and carries
      `brew upgrade` as *its* update command, next to the install command
      rather than in a separate paragraph.
- [x] The README never tells a Homebrew user to run `truthboard update`.
- [x] Source builds keep saying what they already say — `update` refuses
      them by design. Sharpened while writing it: only a *checkout* build
      reports `dev` and is refused; `go install …@latest` is proxy-fetched,
      carries its module version, and updates like a release install. The
      first draft of the fix got this backwards.
- [x] No version number is written anywhere it can rot (tb-02da's rule).
- [x] The detached-board caveat survives: an updated binary does not restart
      running boards.
