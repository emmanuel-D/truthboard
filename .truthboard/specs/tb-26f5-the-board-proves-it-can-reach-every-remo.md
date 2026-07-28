---
id: tb-26f5
title: The board proves it can reach every remote before it serves
branch: '*/tb-26f5-*'
paths:
    - docker/entrypoint.sh
    - cmd/truthboard/**
    - internal/web/**
    - docs/deploy.md
epic: deploy
priority: 1
type: story
---

## Goal

Deploying a private multi-repo hub took five redeploys, and every
failure was a credential problem the board could have named on the
first boot. Instead each one surfaced at the moment of use — a raw
`git clone` failure, repeated once per container restart, with the
actual cause (a token missing one permission) buried in a wall of
identical output.

Run a preflight before serving. One pass, one clear verdict, naming the
remote and the operation that failed rather than echoing git.

Concretely, these are the failures seen in the field:

- `GIT_CONFIG_COUNT=2` with no `GIT_CONFIG_VALUE_1` — every git call
  aborts with "missing config value", nowhere near the cause
- a token with `Code: Push` but not `Code: Download` — hub clone 403s
- no credential at all on a private hub — GitLab answers with the same
  "access denied" text it uses for a *wrong* credential, so the operator
  hunts a bad token when none was supplied
- an edit token armed on a clone with read-only push credentials — the
  board starts happily and the first person to save a story gets a 500

## Acceptance

- [ ] Before serving, the board checks `GIT_CONFIG_COUNT` against the
      `KEY_n`/`VALUE_n` pairs actually present and names any index
      missing a half
- [ ] It `ls-remote`s the hub and reports failure as an unreachable
      remote with a docs pointer, not as raw git output
- [ ] It `ls-remote`s every spoke in the manifest and names precisely
      which ones are unreachable, rather than failing them one by one
      during sync as "no branches found"
- [ ] With an edit token set, it verifies push access to the hub at boot
      and warns loudly when the credential is read-only
- [ ] Failure exits non-zero with a single actionable message, so a
      restart loop repeats a diagnosis rather than a stack of git errors
- [ ] A reachable deploy still boots with no new noise
