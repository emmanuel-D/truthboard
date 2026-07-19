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

Agents working in a spoke don't need the hub cloned — they need the hub's
*board*. Point them at a shared board (see [deploy.md](deploy.md)) and its MCP
surface, or give them a hub checkout for CLI use. Either way the working
agreement is the same one `truthboard adopt` writes: get the brief, work on a
branch containing the spec id, end every commit with the `Spec:` trailer —
in whichever repo the work belongs.

## Current limits

- Forge enrichment (PR states, claims-vs-proof, CI verdicts) applies to the
  hub only for now; spoke landings are proven by git alone.
- A spec is done when its trailer lands in *any* repo of the workspace.
  Declaring that a story must land in *several* repos (`repos:` intent with
  per-repo evidence) is the next story on the board: `tb-c512`.
