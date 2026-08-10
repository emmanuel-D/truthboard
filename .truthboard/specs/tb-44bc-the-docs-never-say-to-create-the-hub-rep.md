---
id: tb-44bc
title: The docs never say to create the hub repo before adopting a workspace
owner: emmanuel
branch: '*/tb-44bc-*'
paths:
    - README.md
    - docs/multi-repo.md
epic: ship-readiness
priority: 1
type: bug
---

## Goal

The README's Quick start is `cd your-project` — a repo that already exists.
Its multi-repo section (`README.md:395`) then explains the hub, prints a
`workspace.yml`, and describes spoke wiring **without ever showing a
command**: no `init --workspace`, no hint that the hub is a repository you
usually have to create. A reader adopting a workspace gets a YAML file and
is left to infer the invocation.

`docs/multi-repo.md:19` has the command but assumes the gap away — "from
the repo that should carry intent" reads as *pick one*, and the following
line describes scaffolding into a repo that already exists.

Yet the single most common multi-repo layout is a workspace folder holding
N project checkouts and no planning repo at all, so the true first step is
`mkdir hub && cd hub && git init` — a step neither document contains.
[[tb-a4ab]] already established what happens when it is skipped: the wiring
lands, init exits 0, and the adopter walks into a wall on their first
derived status. That fix made the *tool* say it; the docs still do not, so
the warning is the only place the requirement appears — the reader learns
their layout was wrong after committing to it.

Both documents should show the whole opening move, hub creation included,
for the layout people actually have.

## Acceptance

- [ ] The README's multi-repo section shows a runnable adoption sequence —
      creating and `git init`-ing the hub, then `init --workspace` — before
      it shows the manifest
- [ ] `docs/multi-repo.md`'s quick start opens with the hub not existing yet,
      and names the sibling-directories layout as the common case
- [ ] Both say why the hub must be a git repository, in one line: every
      derived status starts from git history
- [ ] The existing case — an established repo adopted as the hub — stays
      documented and is not displaced by the new one
- [ ] No document claims a hub can be the workspace parent directory unless
      that directory is itself a repository
