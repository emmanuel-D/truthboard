---
id: tb-9693
title: 'Install ergonomics for the public flip: install.sh and a brew tap'
owner: emmanuel
branch: '*/tb-9693-*'
paths:
    - install.sh
    - .github/**
    - README.md
    - docs/**
epic: ship-readiness
priority: 3
---

## Goal

Blocked on the public flip (unauthenticated release downloads 404 while the repo is private) — everything here can be built and tested now, and goes live the day the repo flips. `curl | sh` and `brew install` are the two gestures people actually try first: an `install.sh` at the repo root (resolves latest tag via the releases/latest redirect — same pattern deploy.md now uses — picks os/arch, verifies against checksums.txt, installs to /usr/local/bin or ~/.local/bin without sudo), and a homebrew tap repo (emmanuel-D/homebrew-truthboard) whose formula the release workflow bumps on every tag.

## Acceptance

- [ ] `install.sh` installs the correct tarball for darwin/linux × amd64/arm64, verifies the checksum, and lands `truthboard` on PATH; refuses politely on unsupported platforms.
- [ ] The script is testable pre-flip (e.g. authed download or fixture mode) and CI-linted (shellcheck).
- [ ] Release workflow updates the brew formula (version + sha256s) on tag push.
- [ ] README install section: curl | sh one-liner and brew tap, replacing the go-install-first story.
