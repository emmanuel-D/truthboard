---
id: tb-fd0b
title: Build-time git ignores the deploy platform's runtime GIT_CONFIG_*
branch: '*/tb-fd0b-*'
paths:
    - Dockerfile
    - docs/deploy.md
priority: 1
type: bug
---

## Goal

Coolify (and platforms like it) inject every configured environment
variable into the image build as `ARG`s. A private multi-repo hub
configures its credential through git's config-from-environment
(`GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_n` / `GIT_CONFIG_VALUE_n`), so
those land at build time too — where they have no business being read.

Git validates the *whole* `GIT_CONFIG_COUNT` set on every invocation
before doing any work. So one missing or mistyped `GIT_CONFIG_VALUE_n`
takes down `git config --system safe.directory '*'` in the runtime
stage, and the build dies with exit 128 pointing at `apk add`:

```
error: missing config value GIT_CONFIG_VALUE_1
fatal: unable to parse command-line config
```

A credential typo should fail the board's clone at runtime with a
message about credentials — not the image build with a message about
package installation. Pin build-time git to zero injected config so the
image builds identically whatever the platform passes in.

## Acceptance

- [ ] The runtime stage's `git config --system` call runs with
      `GIT_CONFIG_COUNT=0`, so it cannot read injected config
- [ ] `docker build` succeeds with `GIT_CONFIG_COUNT=2` and a missing
      `GIT_CONFIG_VALUE_1` in the build environment
- [ ] `safe.directory` is still set in the built image
- [ ] deploy.md warns that platforms which turn env vars into build args
      also bake tokens into image metadata, and says to scope variables
      to runtime where the platform allows it
