# Multi-repo workspaces

Real projects span repositories — an API, a web app, an infra repo. Truthboard
handles this with a **hub-and-spokes** model:

- **Intent lives in one hub repo.** The hub is the repo carrying `.truthboard/`:
  every spec, epic, sprint, and the workspace manifest itself. A product story
  ("password reset flow") doesn't belong to `api` or `web`, so it isn't filed
  in either — it lives in the hub, and the id namespace and `needs:`
  dependencies work across repo boundaries for free.
- **Proof is gathered from every spoke.** Branches, trailers, merges, and
  reverts in each declared repo feed the same derivation rules as ever.
  Statuses stay derived from git, never typed — now from N gits.

A repo without a manifest is simply a workspace of one: nothing changes.

## Quick start

The hub is a git repository, and most workspaces do not have one yet: the
usual starting point is a folder of checkouts — `api/`, `web/`, `infra/` —
with no planning repo among them. So the first move is to make one, next to
the repos it will watch:

```sh
cd ~/dev/acme                 # the folder holding api/ and web/
mkdir hub && cd hub
git init
```

`git init` is not ceremony: every status truthboard reports is derived from
git history, so a hub with no history has nothing to derive from. `init`
warns when it scaffolds into a directory that is not a repository, and
`audit` there tells you the same — but reading it after you have committed
to a layout is late.

With that in place, scaffold the hub in one command:

```sh
truthboard init --workspace api=git@github.com:acme/api.git web=git@github.com:acme/web.git
```

This writes a validated `.truthboard/workspace.yml`, creates
`.truthboard/specs/`, and runs the same agent wiring as `init --agents` —
which, because the manifest now exists, already includes the multi-repo
decomposition guidance. Add `--path name=../checkout` for spokes with local
checkouts (alone or alongside a `name=remote` pair). Re-running with new
pairs merges them into the existing manifest; an existing entry is never
rewritten — change one by editing the file, like any intent.

Every spoke with a local checkout is wired in the same run (see [Spoke
adoption](#spoke-adoption)), so adopting a workspace costs exactly what
adopting a single repo costs.

If a repo in the workspace is already the natural home for intent — a
planning repo, or the one everybody has checked out — run the same command
inside it instead and skip the `mkdir`. The one thing that cannot be the
hub is the workspace *folder*, unless it is itself a repository: `init`
will write the wiring there and nothing will derive from it.

Until a spoke has a local copy — a declared `path:` or the clone the board
server makes (`truthboard ui --detach`) — `truthboard audit` reports that
spoke as unreadable, by name. That is the board being honest about what it
can see, not broken.

## The manifest

Declare spokes in `.truthboard/workspace.yml` in the hub — the repo list is
intent, so it is versioned and reviewed like any spec:

```yaml
repos:
  api:
    remote: git@github.com:acme/api.git
    integration: main
  web:
    remote: git@github.com:acme/web.git
    integration: develop
  infra:
    remote: git@github.com:acme/infra.git
    path: ../infra        # optional: use a local checkout when present
```

- `remote` — what the board server clones and fetches.
- `integration` — the branch landings are proven against. Optional: when
  omitted, the same activity election as the hub applies.
- `path` — a local checkout, relative to the hub root. When it exists it is
  used directly (handy for side-by-side checkouts on a laptop); otherwise the
  server's managed clone is used.

Repo names label everything: branches render as `api:feature/tb-1234-…`,
evidence reads `work landed on api:main`.

## How proof flows

- The **board server** keeps a mirror clone per spoke under the hub's
  `.git/truthboard/spokes/` and runs one sync loop per repo. A spoke that
  hasn't been fetched yet — or whose fetch fails — is reported by name in the
  sync headers and on the page; the freshness timestamp is the *oldest* fetch
  across the workspace, so a stale spoke can never hide behind a fresh hub.
- The **audit itself never clones** (it is read-only by doctrine). Running
  `truthboard audit` on a hub uses declared `path:` checkouts and any clones
  the server has already made; a spoke it cannot see becomes a loud finding
  on the board, never a silent omission.
- **Linking is unchanged** in every repo: the `Spec: tb-1234` trailer, the id
  in a branch name, or the spec's branch glob. A trailer landing on a spoke's
  integration branch flips the spec to done exactly as a hub landing would,
  and active work in *any* repo outranks a landing in another.
- **Scope paths** may target a spoke with a repo prefix: `api:src/auth/**`.
  Unprefixed patterns keep applying to the hub only.

## Spoke adoption

An agent works in the spoke, but intent lives in the hub — so a spoke that
is watched for proof and wired for nothing else is the worst case available:
the hub looks correctly set up while the agent doing the work has no board
to read and no trailer to write. `init --workspace` therefore wires each
spoke that declares a `path:`, in the same run that declares it:

- **MCP registration** (`.mcp.json` and `.vscode/mcp.json`) carrying the hub
  as its argument — `["mcp", "../hub"]`, computed relative from the spoke, so
  the committed file stays portable. `get_brief` and `next_spec` then work
  from the spoke's directory with no further setup. A bare `["mcp"]` would
  serve the spoke, which has no specs in it.
- **The working agreement** in `AGENTS.md` (imported by `CLAUDE.md`), in its
  spoke form: same loop, but it says where intent lives instead of pointing
  at `.truthboard/specs/*.md` files that are not there.
- **The commit-msg nudge**, with `--hooks`. This is where it earns the most:
  a trailerless commit in a spoke is shadow work you find out about later.

A spoke keeps whatever it already had — other MCP servers, existing
`AGENTS.md` prose, someone else's commit-msg hook — and re-running changes
nothing. No `.truthboard/` directory is ever created in a spoke: that would
make it a second, competing hub.

Wiring only counts if the repo will keep it, so adoption checks each file it
writes against that repo's own ignore rules and says so on the spot when one
is excluded — naming the rule and the line, with an exception that works for
that pattern's shape. A `.vscode/*` rule needs only `!.vscode/mcp.json`,
while a `.vscode/` rule excludes the directory itself and needs
`!.vscode/`, `.vscode/*` and `!.vscode/mcp.json`, because git never descends
into an excluded directory. It is warn-only: truthboard writes the file and
tells you what it knows, and editing `.gitignore` stays your call. Already
tracked files are never warned about — ignore rules do not apply to them.

Spokes that cannot be wired are named, never skipped in silence — no
`path:` declared, not checked out yet, or a path holding a checkout of a
different repository. Adoption never clones; check the repo out, then re-run
(`truthboard init --workspace` with no new pairs re-applies the wiring).
`--no-spokes` wires the hub alone.

### Staying wired

Wiring a spoke once is not the same as a spoke staying wired: a fresh clone,
a hand-edited `.mcp.json`, or a spoke declared after the last setup run each
leave a repo watched for proof whose agents have no board. So it is derived
like everything else — `truthboard audit` reports **unwired spokes** as
drift, naming the repo, what is missing, and the command that fixes it:

```
Unwired spokes (1): checked out and watched for proof, but agents there have no board
  - web (../web): no MCP registration (agents here have no board), no working
    agreement in AGENTS.md — re-run `truthboard init --workspace` in the hub
```

Wiring that is present but *ignored* is reported here too, as its own
finding: the file exists on the machine running the audit, so nothing looks
wrong, while a teammate's clone of that spoke has no board. Its remedy is the
`.gitignore` exception, never the re-run — re-running rewrites a file the
repo already has and still throws away.

The audit detects, it never wires. A spoke with **no local copy** is not a
wiring finding — that is the unreadable-spoke report above, and saying the
same thing twice in two vocabularies teaches people to skim both. The
commit-msg nudge is opt-in (`--hooks`), so its absence is never a finding on
its own; it is named only when a spoke is already being reported and the hub
has one.

Agents that work from a **shared board** instead of a hub checkout (see
[deploy.md](deploy.md)) need no local wiring at all — same agreement, same
trailer, in whichever repo the work belongs.

## Stories that must land in several repos

Git cannot prove the absence of work it never knew was intended: without
declared intent, a story touching `api` and `web` looks done the moment its
trailer lands in `api`. When a story genuinely must land in more than one
repo, declare it:

```yaml
repos: [api, web]      # or include the hub itself: [hub, api]
```

`hub` is a reserved name for the repo carrying `.truthboard/`; spokes go by
their manifest names. With `repos:` declared:

- **Done requires all of them** — the trailer landed on the integration
  branch of every declared repo.
- **Evidence is per-repo chips**, so a partial landing says exactly what is
  missing: `api ✓ landed · web — no branch yet`. A stalled or in-flight
  branch in the missing repo shows as such, and active work anywhere still
  outranks landings.
- **A revert in any declared repo regresses the story**, evidence naming
  the repo.
- **Unknown names fail loudly**: every write path (CLI, MCP, web editor)
  validates against the manifest, and a hand-edited spec naming an
  undeclared repo becomes a drift finding (`Unknown repos`) — it can never
  derive done.

### Declare `repos:`, or split the story?

`repos:` is the mechanism; per-repo decomposition is often the better
practice. One spec per provable landing — `tb-1234` (api half) and
`tb-77ab` (web half) under the same epic, ordered with `needs:` — keeps
every status maximally honest and lets each half ship, review, and regress
on its own. Reach for `repos:` when the story is genuinely one promise
that is only true once every repo has it (a lockstep protocol change, a
coordinated rename); reach for splitting when the halves have independent
value or different owners. An agent picking up a fat cross-repo brief can
do the splitting itself over MCP (`create_spec` + `needs:`).

### A worked example: one phone story, two provable landings

A PO on the road creates a story from the shared board (edit-token flow):

> **tb-9f3e — Password reset flow** · p1
> Users can request a reset link and set a new password.

The workspace watches `api` and `web`. An agent at home runs `next_spec`,
and the brief tells it this hub gathers proof from `api, web` and to split
before coding. It does, over MCP:

1. `update_spec` — narrow the original into the first half:
   `{"id": "tb-9f3e", "title": "Password reset — api", "repos": ["api"], "epic": "password-reset"}`
2. `create_spec` — the second half, blocked on the first:
   `{"title": "Password reset — web", "repos": ["web"], "epic": "password-reset", "needs": ["tb-9f3e"], "body": "## Goal\n…"}`

No orphan remains — the original *became* the api half, so no card sits
planned forever waiting for a branch that will never come. The board now
shows two stories under the `password-reset` epic; `next_spec` hands out
only the api half (the web half is *waiting on tb-9f3e*, and the readiness
rule works across repos because ids are global).

The agent works the api repo on `feature/tb-9f3e-reset-api`, trailer
`Spec: tb-9f3e`, merges — the api story derives done from the spoke
landing, the web story becomes startable, and whoever (or whatever)
picks it up next repeats the loop in the web repo. Both stories flip to
done purely from git; the epic's rollup shows combined progress the whole
way. The PO never needed to know there were two repos involved.

## Forge enrichment per repo

With `--forge` (or without `--no-forge` on `audit`), every repo in the
workspace is enriched by its *own* forge — gh/glab auto-detect from each
repo's remote, so a GitHub hub can watch a GitLab spoke:

- A spoke branch with an open PR derives **in-review**, evidence naming
  the PR and the repo (`api:feature/… — PR #7 open`).
- Claims-vs-proof runs per repo against that repo's tracker; claim
  subjects carry the repo name (`api:#12`).
- Red CI on a spoke landing flips the spec that landed there to
  **regressed** — including the per-repo landings of `repos:` stories —
  with the repo named in the evidence.
- A spoke whose forge is unreachable or unauthenticated keeps its
  git-only derivation and shows a visible note on the board and in the
  workspace header — degraded, never silent, never an error.

A spoke branch is only ever matched against its own repo's PRs; a hub PR
that happens to share a branch name proves nothing about a spoke.
