---
id: tb-5266
title: A credential never reaches the logs
branch: '*/tb-5266-*'
paths:
    - docker/entrypoint.sh
    - internal/**
epic: deploy
priority: 1
type: bug
---

## Goal

`docker/entrypoint.sh` echoes `$REPO_URL` verbatim before cloning. The
documented way to deploy a single private repo is to embed a token in
that URL — so following the docs prints the token into the deploy log,
where platforms archive it and operators paste it into bug reports.

Git's own errors have the same problem: it reports the URL it failed on,
credentials included.

Anything that prints a remote URL redacts its userinfo first.

## Acceptance

- [ ] The entrypoint's clone message shows the URL with any
      `user:password@` replaced by a redaction marker
- [ ] Errors the board surfaces that carry a remote URL are redacted the
      same way, including git's own output when it is passed through
- [ ] A URL with no credential is printed unchanged
- [ ] Covered by a test that a token in `REPO_URL` never appears in
      output
