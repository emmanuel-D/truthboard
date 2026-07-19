---
id: tb-9cf1
title: 'Docs catch up with reality: version-proof install line, README status current'
owner: emmanuel
branch: '*/tb-9cf1-*'
paths:
    - docs/**
    - README.md
epic: ship-readiness
priority: 2
---

## Goal

Two docs spots hardcode a release that is no longer current, and one of them will go stale again on every future tag. deploy.md's install one-liner names the `v0.5.0` tarball (latest is v0.6.0, and the `/releases/latest/download/` URL breaks whenever the embedded version doesn't match the actual latest); the README Status section still says `v0.5.0` and predates the multi-repo workspaces feature. Fix the staleness class, not just the instance: resolve the tag dynamically in the install snippet so no future release invalidates the docs again.

## Acceptance

- [ ] deploy.md installs the latest release without naming a specific version (tag resolved at run time), and the snippet works against the real release asset naming.
- [ ] README Status reflects the current release line and mentions multi-repo workspaces.
- [ ] No other doc names a stale release tag (grep -rn "v0\.[0-9]" over README + docs is clean of stale references).
