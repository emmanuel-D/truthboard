---
id: tb-6452
title: An expired tap token cannot turn a shipped release red
owner: emmanuel
branch: '*/tb-6452-*'
paths:
    - .github/workflows/release.yml
epic: ship-readiness
priority: 2
type: bug
---

## Goal

The release workflow's tap step says in its own comment that "a release
must never fail over brew ergonomics", and it keeps that promise for
exactly one case: an empty `HOMEBREW_TAP_TOKEN`, which it detects and
skips. Every other way the token can go wrong — expired, revoked, scoped
to the wrong repo, tap renamed — reaches `git clone` under
`set -euo pipefail` and fails the job.

By then the release exists: archives built, checksums uploaded, GitHub
release created. A red run against a shipped tag reads as "the release
failed" to everyone who looks at it, and the actual fault is a formula
that needs one manual commit.

PATs expire. This will happen, quietly, months after the secret is set.

## Acceptance

- [ ] A tap bump that fails for any reason leaves the release job green
- [ ] The failure is loud where someone will see it: a `::warning::` annotation on the run, naming the likely cause and what to do by hand
- [ ] An empty token still says so and skips, as it does today
- [ ] A formula that is already current still short-circuits without an empty commit
- [ ] The happy path is unchanged: a valid token still clones, regenerates the formula, commits, and pushes
