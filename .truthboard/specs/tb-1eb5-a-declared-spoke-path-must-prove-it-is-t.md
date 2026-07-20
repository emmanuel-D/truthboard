---
id: tb-1eb5
title: A declared spoke path must prove it is the declared repo
owner: emmanuel
branch: '*/tb-1eb5-*'
paths: [internal/workspace/**, internal/audit/**]
epic: multi-repo
priority: 2
---

## Goal

`Workspace.Resolve` accepts a declared `path:` on the single evidence that
the directory exists (`workspace.go:117`). Nothing checks that the checkout
there is actually the repo named by `remote:`. Point a spoke at the wrong
sibling directory — a typo, a stale checkout, two clones of different
generations — and the board reads proof from the wrong repository and
reports it with full confidence.

That is the one failure mode this product exists to prevent. Every other
wrong answer in truthboard is loud (`unreadable` spokes, drift findings,
the "no local copy" error); this one is silent and looks exactly like
truth. It is also more likely than it sounds: the LetTalk workspace
carries `lettalk-web`, `lettalk-web-v2` and `lettalk-connect-web` side by
side, and the correct `web` spoke is the *second* of those pointing at the
*third*'s remote. `--path web=../lettalk-web` would have been accepted in
silence.

Fix: when a spoke declares both `remote:` and `path:`, compare the
checkout's `origin` to the declared remote and refuse the mismatch loudly,
naming both URLs. Comparison must normalise the forms that mean the same
repository — scp-style vs https, trailing `.git`, trailing slash, embedded
credentials — or the check will cry wolf on correct setups and get
disabled. A path-only spoke has nothing to compare against and stays
allowed; the read-only doctrine is untouched, since this only ever reads
`git remote get-url` and never mutates or clones.

## Acceptance

- [ ] A spoke whose declared path holds a checkout of a different remote is
      refused with both URLs named, in the audit and on the board
- [ ] `git@host:acme/api.git`, `https://host/acme/api.git`,
      `https://host/acme/api` and a credentialed URL all compare equal
- [ ] A path-only spoke (no `remote:`) is unaffected
- [ ] A spoke with no local checkout keeps today's "no local copy" message
- [ ] The audit still never clones, fetches, or writes to a spoke
