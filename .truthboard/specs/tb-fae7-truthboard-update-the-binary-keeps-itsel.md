---
id: tb-fae7
title: 'truthboard update: the binary keeps itself current'
owner: emmanuel
branch: '*/tb-fae7-*'
paths:
    - internal/selfupdate/**
    - internal/web/**
    - cmd/truthboard/**
    - README.md
epic: ship-readiness
priority: 1
---

## Goal

Today a stale binary is invisible: detached boards keep serving whatever
build they were started with, and nothing ever says "you're behind". A
pilot who installed v0.2.0 will still be on v0.2.0 in a month.

`truthboard update` closes the loop:

- Resolves the latest GitHub release (via `gh` when available — works
  while the repo is private — else the public API), compares to the
  running version.
- `--check` only reports. Without it, downloads the right
  `truthboard_<tag>_<os>_<arch>.tar.gz`, verifies its sha256 against the
  release's checksums.txt (refusing when checksums are missing), and
  atomically replaces the current executable.
- Source builds (version `dev`) are never silently replaced: report the
  latest release and point at `git pull && go install`.
- After updating, remind that detached boards keep running the old
  binary until `truthboard stop` + `ui --detach`.
- The board page footer shows the serving binary's version, so a stale
  board is visible at a glance.

## Acceptance

- [ ] `truthboard update --check` reports current vs latest without
      touching anything; up-to-date says so and exits 0
- [ ] `truthboard update` downloads, checksum-verifies, and atomically
      replaces the executable; a checksum mismatch aborts with the old
      binary intact
- [ ] dev builds refuse self-replacement with a clear source-build
      message
- [ ] after a successful update the output nudges restarting detached
      boards
- [ ] the web board footer shows the binary version serving it
- [ ] README install section documents truthboard update
