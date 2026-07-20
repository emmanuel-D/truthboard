---
id: tb-569e
title: go install builds report their module version, not dev
owner: emmanuel
branch: '*/tb-569e-*'
paths: [cmd/truthboard/**, internal/selfupdate/**]
epic: ship-readiness
priority: 2
---

## Goal

`version` in `cmd/truthboard/main.go` is stamped by the release workflow's
`-ldflags "-X main.version=<tag>"`. `go install` applies no such flags, so
the third install path the README documents produces a binary that calls
itself `dev`. Verified against the public proxy the moment the repo went
public: `go install github.com/emmanuel-D/truthboard/cmd/truthboard@latest`
downloaded v0.8.3 and then reported `truthboard dev`.

Two consequences, and the second is the sharp one. The board footer says
`dev build (source)` for someone running a tagged release. And
`truthboard update` deliberately refuses to replace a dev build — it reads
that as a working copy someone is developing against — so a `go install`
user is on a version-less binary that will never self-update and never
tells them a newer release exists. The install path most likely to be used
by Go developers is the one that silently opts them out of updates.

The fix needs no build-system change: for `go install pkg@version`, the
module version is recorded in the binary and readable via
`runtime/debug.ReadBuildInfo().Main.Version`. When the ldflags value is
absent, fall back to that. A true local `go build` still has no module
version (`(devel)`), so it must keep reporting `dev` and keep its
self-update immunity — that distinction is the whole point and must not
be flattened.

## Acceptance

- [ ] A binary from `go install …@vX.Y.Z` reports `vX.Y.Z` from `version`
      and in the board footer
- [ ] A binary from a local `go build` still reports `dev` and is still
      refused by `truthboard update`
- [ ] A release binary built with ldflags is unchanged — the stamped value
      always wins over build info
- [ ] `truthboard update` offers a newer release to a `go install` user
      rather than declining as it does today
