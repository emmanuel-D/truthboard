---
id: tb-c847
title: Deploy docs cover private multi-repo hubs and read exposure
owner: emmanuel
branch: '*/tb-c847-*'
paths: [docs/**]
epic: ship-readiness
priority: 1
---

## Goal

The first attempt to deploy a real private multi-repo hub to Coolify hit
two gaps in `docs/deploy.md`, both of which the docs actively set someone
up for.

**Private spokes have no documented credential path.** `REPO_URL` with an
embedded token is documented and covers the hub, so a reader reasonably
concludes private repos are handled. They are not: each spoke is cloned
separately from the remote in `.truthboard/workspace.yml`
(`internal/web/sync.go`), a committed file that must never carry a
credential. The hub clones, every spoke fails, and the board comes up
loud but empty. Verified against real private GitLab spokes: `fatal:
could not read Password for 'https://oauth2@gitlab.com'`.

The remedy needs no code — git's `GIT_CONFIG_COUNT`/`KEY`/`VALUE`
environment config can rewrite the host prefix to carry a read-only
token, leaving the manifest clean. Verified end to end through the sync
loop: the spoke mirror-cloned and the audit reported `workspace: connect
(main)`.

**"Read-only" is being read as "safe to expose."** The intro promises a
shared board is read-only beyond loopback, which is true of *writes* and
says nothing about who may look. There is no read authentication at all
(`internal/web/server.go` gates writes only), so anyone who reaches the
URL sees every branch name, commit subject, author and drift finding —
for a private repo, that is its activity published at whatever address
the domain points to. Someone following Option C to the letter gets that
outcome without ever being warned.

Both are documentation failures rather than product bugs, and the second
is the one that can cause real harm.

## Acceptance

- [ ] `docs/deploy.md` documents credential injection for private spokes,
      with a worked GitLab and GitHub example and the read-only scope to
      use, and states that the manifest must never carry a credential
- [ ] The first-boot window where spokes read as unreadable while mirrors
      clone is described, so it is not mistaken for a broken deploy
- [ ] The read-exposure limit is stated where "read-only" is introduced,
      not only in a later section, and names the mitigations (proxy auth,
      IP allowlist, private network)
- [ ] The can/cannot section distinguishes what a visitor may change from
      what a visitor may see
