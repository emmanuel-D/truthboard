---
id: tb-4f55
title: truthboard update refuses to write behind Homebrew's back
branch: '*/tb-4f55-*'
paths:
    - internal/selfupdate/**
    - README.md
epic: ship-readiness
priority: 1
---

## Goal

`replaceExecutable` resolves symlinks before swapping the binary, so on a
Homebrew install `truthboard update` writes straight into the Cellar. Brew's
metadata still reports the old version, `brew list --versions` disagrees with
`truthboard version`, and the next `brew upgrade` or `reinstall` silently
reverts the update. The user is left believing they are current when they are
one `brew upgrade` away from going backwards.

tb-8222 documented the trap. Documentation is the weakest possible fix for a
command that can detect the situation itself: the binary knows where it lives.

A source build is already refused for exactly this reason — `update` will not
overwrite a working copy someone is developing against. A brew keg deserves
the same treatment: refuse, and name the command that does the job properly.

## Acceptance

- [x] `truthboard update` on a Homebrew-managed binary does not replace it,
      and exits 0 — this is guidance, not a failure, matching how a source
      build is already handled.
- [x] It names the working command (`brew update && brew upgrade truthboard`)
      and says in one line why brew has to do it.
- [x] `truthboard update --check` gives the same brew advice rather than
      telling the user to run a command that will refuse.
- [x] An already-current brew install still just says it is up to date — no
      warning where there is nothing to do.
- [x] Detection is structural, not a substring: the resolved executable must
      sit at `…/Cellar/<formula>/<version>/bin/<exe>`. A path that merely
      contains "Cellar" is not a brew install, and neither Linuxbrew's prefix
      nor a custom one may be excluded.
- [x] Detection runs on the *resolved* path, since `/opt/homebrew/bin/…` is a
      symlink into the Cellar — that indirection is the whole bug.
- [x] Every other install path is untouched: script installs, tarballs and
      `go install …@latest` still update in place.
- [x] The README's warning becomes a statement of what the command does now.

Verified beyond unit tests, against the live release feed: a v0.12.0-stamped
binary in a real keg layout, invoked through the `bin/` symlink brew creates,
printed the brew advice and left the keg byte-identical; the same binary in
an ordinary directory really downloaded and became v0.12.2. The negative
control matters — it is what proves the refusal is a decision and not a
silently broken update path.
