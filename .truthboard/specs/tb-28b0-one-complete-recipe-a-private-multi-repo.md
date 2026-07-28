---
id: tb-28b0
title: 'One complete recipe: a private multi-repo hub with editing, on Coolify'
branch: '*/tb-28b0-*'
paths:
    - docs/deploy.md
epic: deploy
priority: 2
type: task
---

## Goal

deploy.md covers private hubs and editing separately, and the two
interact in a way that is only obvious once you have hit it: arming
`TRUTHBOARD_EDIT_TOKEN` means the hub clone now needs *push*
credentials, while the spokes still need only read. That turns the
single credential rule into two, and a read-only token on the hub
becomes a 500 on the first save rather than a boot failure.

Write the whole thing down once, as a recipe an operator can follow
top to bottom without deriving anything.

## Acceptance

- [ ] One worked example: private hub, private spokes, editing armed —
      the complete environment table in a single block
- [ ] Explains the two-rule `insteadOf` form and why: git resolves by
      longest match, so a hub-specific rule overrides the group-wide
      read-only one, and `VALUE_n` must match `REPO_URL` exactly
- [ ] States which forge permissions each token needs, in the forge's
      own vocabulary (GitLab fine-grained `Code: Download` for the
      spokes, `Download` + `Push` for the hub)
- [ ] Says to verify each token with `git ls-remote` before redeploying
- [ ] A troubleshooting table mapping the real error strings operators
      see — GitLab's "access denied", `[Code: Download]`, "missing
      config value" — to their cause
- [ ] Covers read exposure: a shared board is world-readable unless the
      platform's basic auth is enabled, and the edit token gates writes
      only
