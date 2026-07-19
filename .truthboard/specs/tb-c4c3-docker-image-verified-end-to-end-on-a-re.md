---
id: tb-c4c3
title: Docker image verified end-to-end on a real Docker engine
owner: emmanuel
branch: '*/tb-c4c3-*'
paths:
    - Dockerfile
    - docker/**
    - docs/deploy.md
    - .github/**
epic: ship-readiness
priority: 3
---

## Goal

The tb-146e Dockerfile shipped verified by cross-compiling the exact build line and driving the entrypoint directly — but `docker build` itself has never run (no Docker on the dev Mac). Close that gap where Docker does exist: a CI job (or a one-off run on a Docker machine) that builds the image, starts it against a fixture repo via REPO_URL, and asserts the board answers — reads 200, writes 403 beyond loopback, sync headers present. Once green, consider publishing the image on tag push (ghcr.io) so `docker run` needs no local build; publishing may be split out if the registry decision isn't ready.

## Acceptance

- [ ] `docker build` succeeds from a clean checkout (proven in CI or a documented run).
- [ ] The container serves a board for a REPO_URL-cloned fixture: / and /api/board return 200, writes return 403, X-Truthboard-Sync-* headers present.
- [ ] Entrypoint knobs exercised: PORT, FETCH=0 (webhook-only), mounted /repo variant.
- [ ] docs/deploy.md Docker section states the verified image/build status (no more untested caveat).
