---
id: tb-146e
title: 'Shared board deploys anywhere: Dockerfile + server install docs'
owner: emmanuel
branch: '*/tb-146e-*'
paths:
    - Dockerfile
    - .dockerignore
    - docker/**
    - docs/**
    - README.md
epic: ship-readiness
priority: 1
---

## Goal

A team can put a read-only shared board on any server — EC2, a VPS, or a
PaaS like Coolify — in minutes, from the docs alone. The repo ships a
production-ready Dockerfile and a deploy guide covering the three common
shapes: plain binary under systemd, Docker, and Coolify.

## Acceptance

- [ ] `docker build` on the repo produces a small image that serves the board for a repo cloned/mounted into the container; the runtime image contains git (statuses are derived from it).
- [ ] `docs/deploy.md` documents: binary + systemd install, Docker run, and Coolify setup, including `--host 0.0.0.0`, `--fetch` polling vs `--webhook-secret` push mode, and the reverse-proxy/SSE buffering caveat.
- [ ] The guide states plainly that a board served beyond loopback is read-only (no auth story yet) and that intent editing happens from a clone.
- [ ] README links to the deploy guide from the features/docs section.
