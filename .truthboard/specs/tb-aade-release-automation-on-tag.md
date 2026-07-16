---
id: tb-aade
title: Release automation on tag push
owner: emmanuel
branch: '*/tb-aade-*'
paths: ['.github/**', cmd/truthboard/**]
epic: ship-readiness
priority: 2
---

## Goal

v0.1.0's binaries were built by hand in a shell. Pushing a `v*` tag should
produce the release automatically: five platform binaries, checksums, and
generated notes — so releasing never depends on someone's laptop.

## Acceptance

- [x] Pushing a `v*` tag builds darwin/linux (amd64+arm64) and windows/amd64 binaries with the version stamped in (`truthboard version` prints the tag)
- [x] A GitHub release is created with the tarballs, a checksums file, and generated notes
- [x] Verified end-to-end by cutting v0.2.0
