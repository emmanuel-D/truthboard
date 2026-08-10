---
id: tb-d32e
title: A workspace adopts truthboard in one command, not five
owner: emmanuel
branch: '*/tb-d32e-*'
paths:
    - internal/adopt/**
    - internal/workspace/**
    - cmd/truthboard/**
epic: dev-experience
priority: 2
type: story
---

## Goal

Adopting a single repo is one command (`init --agents --hooks`). Adopting a
workspace is five: create a directory, `git init` it, type a `name=remote`
pair *and* a `--path name=../dir` pair for every repo, start the board,
commit. Four of those are constant cost; the third grows with the number of
repos, and it is transcription — the remotes being typed are sitting in the
sibling checkouts' own git config.

That is the shape of the multi-repo onboarding cost, and it lands on exactly
the adopter with the most repos to lose patience over. Worse than the
minutes: the command demands you already understand hub-vs-spoke before it
will accept a single argument, so the model has to be learned from docs
rather than from the tool.

The machinery is already here and already blessed as read-only —
`internal/workspace/workspace.go:168` shells out to `git remote get-url
origin` on a declared path to verify the checkout is the repo you claimed
([[tb-1eb5]]). It reads the remote to check it, but makes you type it first.

Make bare `truthboard init --workspace` in a directory with sibling git
repositories propose the manifest instead of demanding it:

    Found 4 git repositories next to this hub:
      connect  ../lettalk-connect   gitlab.com/wetalk1/lettalk-connect
      server   ../lettalk-server    gitlab.com/wetalk1/lettalk-server
      web      ../lettalk-web-v2    gitlab.com/wetalk1/lettalk-connect-web
      devops   ../lettalk-devops    gitlab.com/wetalk1/lettalk-devops
    Declare all as spokes? [Y/n/edit]

N typed pairs become one keystroke, and the proposed manifest *teaches* the
hub/spoke model at the moment it matters instead of requiring it upfront.

Proposing is the whole point: a silently-declared spoke means the board
gathers proof from a repo nobody meant to watch, and a workspace folder
holds plenty that is not a spoke (`media`, a pitch deck, an unrelated
checkout). Discovery never guesses on the adopter's behalf — it shows, then
asks. And it stays inside the read-only doctrine: reading config files is
not cloning, and adoption still never fetches.

The two remaining ceremonies fold in behind the same command. `git init`
is warn-only today ([[tb-a4ab]]) on the reasoning that creating a repo stays
the adopter's call — right when init runs in a directory that already
exists with content, overcautious when the tool is creating the hub
directory itself. Split those cases. The adoption commit is already exempt
from its own drift findings ([[tb-3d43]]); something you have gone to
trouble to stop reporting as shadow work is something you can safely author.

## Acceptance

- [ ] `truthboard init --workspace` with no repo pairs, in a directory with
      sibling git repositories, lists each candidate with its derived name,
      relative path and redacted remote, and declares nothing until confirmed
- [ ] Confirming writes the same manifest and runs the same spoke wiring as
      hand-typed pairs — `path:` set for every discovered repo, MCP pointed
      back at the hub, no `.truthboard/` in any spoke
- [ ] Declining, or `edit`, leaves no manifest written and no wiring applied
- [ ] Discovery never clones and never fetches; a sibling whose origin
      cannot be read is offered as path-only, never skipped in silence
- [ ] Explicit `name=remote` pairs still work unchanged and suppress the
      prompt; a non-interactive run without `--yes` declares nothing and says
      which flag would have
- [ ] Re-running on an existing manifest offers only repos not already
      declared, and never rewrites an existing entry
- [ ] A hub directory the command creates itself is `git init`-ed without a
      prompt; an existing non-git directory keeps the [[tb-a4ab]] warning
- [ ] `--commit` writes the adoption commit in the hub and in every wired
      spoke, each confined to governed files so it stays exempt under
      [[tb-3d43]]; without the flag the commands are printed, never run
- [ ] `--ui` chains the detached board, so nothing-to-running-board is one
      command
- [ ] Sibling directories that are not git repositories are never offered
