# Truthboard — V1 Concept Document

**Name:** Truthboard (decided — see §10; "GitAgile" and "Driftless" retired)
**Author:** Emmanuel Dadem
**Date:** 2026-07-15
**Status:** Concept — pre-validation
**Supersedes:** GitAgile Technical Specification & Implementation Plan (V2)

---

## 1. One-liner

**Your repo already knows the status. Stop typing it twice.**

Truthboard is a local-first, git-native delivery tracker where humans and AI agents
write down *intent* once — a small markdown spec — and every other piece of project
state (status, progress, reports) is **derived from git reality, never typed by hand**.

## 2. The problem

Every existing tracker — Jira, Linear, Backlog.md — is **input-centric**: it is a
database of claims that humans must keep in sync with reality by hand. That manual
sync is unpaid, hated labor, and it is the root cause of five well-documented
frustrations:

1. **Duplicated work.** Developers re-describe in the tool what they already
   expressed in branches, commits, and PRs. They stop doing it; the board rots.
2. **The visibility paradox.** Teams have more dashboards than ever, yet the 18th
   State of Agile report finds 63% struggle to deliver reliable software and only
   27% say agile helps them deliver value. Dashboards built on stale manual input
   produce confident-looking lies.
3. **Status theater.** Standups become interrogations and story-point rituals
   persist *because the board can't be trusted*. Fix the trust, and the theater
   loses its reason to exist.
4. **AI agents broke the unit of work.** A growing share of code is written by
   agents across multiple sessions. No tracker records the chain
   *spec → agent session → diff → human review → outcome*. Teams cannot answer
   "what did the agents do this sprint, and did anyone check it?"
5. **Stakeholder reporting is manual glue.** PMs ask "what's the status?" because
   the truth (git) is unreadable to them and the readable thing (the board) is
   untrue.

**Root cause, stated once:** the truth about work lives in git; the story about
work lives in a tracker; syncing them is manual. Truthboard deletes the sync.

## 3. Who it's for

| Persona | Pain today | What Truthboard gives them |
|---|---|---|
| **Developer / tech lead** (primary) | Updating tickets, defending status in standups | Writes a 10-line spec (needed anyway to brief an agent), then never touches the tracker again |
| **AI agent** (primary, first-class) | Context rot, no durable record of its work | Reads the spec as its brief; its sessions and diffs are auto-attached to it |
| **PO / EM / stakeholder** (secondary) | Asking for status, distrusting the board | A board and weekly digest that are *provably* derived from repository facts |

V1 targets **small teams (1–8 devs) already using agent-assisted development**,
where the spec-writing habit already exists and there is no entrenched Jira admin.

## 4. Product principles (non-negotiable)

1. **Derived, never typed.** There is no command and no UI affordance to manually
   set a status. If the inference is wrong, you fix the *inputs* (link a branch,
   amend the spec), not the output. This is the product; compromise it and we
   become another Backlog.md.
2. **Files are truth, absolutely.** Specs are plain markdown + YAML frontmatter in
   `.truthboard/` inside the repo, versioned by git. Any cache (SQLite index) is
   disposable and rebuilt from files — never written first.
3. **Local-first, zero-config.** One static binary. `truthboard init` and you're
   running. No server, no account, no daemon required for V1.
4. **Honesty over completeness.** When inference is uncertain, the tool says
   "unknown — here's why" rather than guessing. Trust is the entire value
   proposition; one confident wrong status costs more than ten honest "unknowns".
5. **Agent-native, not agent-gimmicky.** Specs are structured so an agent can be
   pointed at one and start working (`truthboard brief <id>` emits the context
   packet). No embedded LLM calls in V1.

## 5. Object model

### 5.1 The Spec (the only thing a human writes)

One markdown file per unit of work: `.truthboard/specs/042-email-verification.md`

```markdown
---
id: 042            # assigned as <prefix>-<hash> internally to avoid merge collisions
title: Add email verification to signup flow
owner: emmanuel
branch: feature/042-*        # glob; auto-suggested at creation
paths: [frontend-app/src/auth/**]   # optional scope hint, powers creep detection
---

## Goal
Prevent spam accounts by verifying user emails before dashboard access.

## Acceptance
- Verification token expires in 24h
- Unverified accounts cannot log in
```

No status field. No points. No sprint field in V1.

### 5.2 Derived status (the state machine)

| Observed in git | Derived status |
|---|---|
| Spec exists, no matching branch/commits | `planned` |
| Matching branch has commits | `in-progress` |
| PR open referencing spec/branch | `in-review` |
| PR merged to default branch + CI green | `done` |
| Merged but CI red, or merge then revert detected | `regressed` (flagged loudly) |
| No commits on matching branch for N days (default 7) | `stalled` |

Linking signals, in priority order: explicit `Spec: 042` commit trailer →
branch-name glob match → PR body reference. `truthboard link <id> <branch|pr>`
exists as the manual escape hatch for the messy 10% — it fixes the *input*, not
the status.

### 5.3 Drift report (the honesty layer — the killer feature)

`truthboard drift` answers "where is the board lying?":

- **Shadow work:** commits/branches with no linked spec — work nobody promised.
- **Stale promises:** specs with no git activity for N days.
- **Scope creep:** linked diffs touching files far outside the spec's `paths`.
- **Regressions:** reverts or red CI on merged specs.

This is what makes the board *trustworthy*, which is what makes standup-as-status
obsolete.

### 5.4 Digest (the stakeholder killer feature)

`truthboard digest [--since 1w] [--md|--html]` produces a human-readable narrative:
what shipped (merged specs with their goals, not commit messages), what's moving,
what's stalled, what drifted. Pipeable to Slack/email by the user; no built-in
integrations in V1.

## 6. V1 scope (the MVP cut)

**In:** single static Go binary; `init`, `spec new`, `status` (board as terminal
table), `drift`, `digest`, `brief <id>`, `link`; local git history as the only
data source; GitHub PR/CI state via `gh` CLI if present (graceful degradation to
branch/merge inference without it); SQLite as a rebuildable index.

**Explicitly deferred:**

- **V1.1** — agent work ledger (session records attached to specs; MCP server so
  agents read/write specs natively). Deferred only because it needs V1's linking
  to be solid first; it is the second wedge, not an afterthought.
- **V2** — web UI board (read-only first), background daemon, GitLab support.
- **Won't build** (learned from GitAgile V2 spec review): central shared server
  mode (auth/TLS/accounts = SaaS-shaped scope poison), embedded LLM features,
  story points, velocity charts, workflow configuration, manual status of any kind.

## 7. Differentiation

| | Input model | Status source | Stakeholder story | Agent story |
|---|---|---|---|---|
| **Jira / Linear** | Heavy manual | Humans drag cards | Dashboards on stale data | Bolted-on |
| **Backlog.md** | Light manual (markdown) | Humans edit status | None (dev-only) | Good (briefing) |
| **Gitmore / Gitrecap** | None | Git activity | Activity reports, **no intent** | None |
| **Truthboard** | Intent only (10-line spec) | **Derived from git** | Digest linking intent→outcome | Spec as brief + (V1.1) ledger |

The moat is a *refusal*: no manual status, ever. Incumbents can't copy it without
breaking their own data model; Backlog.md would have to remove features to match it.

## 8. Risks and open questions

1. **Inference reliability is the product.** Abandoned branches, hotfixes,
   squash-merges, force-pushes. Mitigation: honest `unknown` state, `link` escape
   hatch, and an acceptance bar — **≥90% of statuses correct with zero manual
   linking on 3 real repos** before any V1 announcement.
2. **Habit change:** devs must write a spec before coding. Mitigation: target
   agent-assisted teams for whom the spec is already the agent brief; make
   `spec new` a 20-second operation.
3. **Monorepos / multi-repo features:** one spec spanning two repos is out of
   scope for V1 — decide the story before V2.
4. **"Surveillance" perception:** drift reports name people's unpromised work.
   Positioning must be *the board stops lying*, not *managers watch you*. Default
   digest aggregates by workstream, not by person.
5. **Is drift-detection alone enough to switch tools for?** Unvalidated — hence §9.

## 9. Validation plan (before Phase 1 code)

1. **Concept pitch:** one page (this doc's §1–2–5) shown to 5–10 tech leads and
   POs on agent-using teams. Kill signal: nobody says "I'd install that this week."
2. **Watering holes:** Backlog.md issues/HN thread, r/ExperiencedDevs, agile
   forums — collect verbatim quotes matching the problem statement.
3. **Wizard-of-Oz drift report:** hand-produce a `drift`-style report for one real
   team's repo and show it to their lead. If the reaction isn't "how did you get
   this?", revisit §5.3.
4. Only then: Phase 1 (spec parser + git scanner + `status`/`drift` CLI) — reusing
   the GitAgile V2 stack decisions (Go, SQLite index, single binary), which remain
   sound.

## 10. Naming — decided: Truthboard

"GitAgile" was retired (SFC Git trademark policy discourages `git-` prefixes;
"Agile" narrows the audience). Availability check on 2026-07-15:

- **Truthboard — selected.** npm, crates.io, Homebrew all free; no meaningful
  GitHub collisions; `truthboard.dev` and `truthboard.io` have no DNS (likely
  registrable — register before announcing anything). Bonus: the name *is* the
  thesis — the board you can trust.
- Driftless — rejected: npm taken (85★ timer library), driftless.dev/.io
  registered.
- Asbuilt — rejected: AsBuiltReport org (178★) occupies the adjacent
  documentation-tooling space.

## 11. Prototype findings (2026-07-15, `prototype/scan.py`)

Ran a pure-git inference prototype (no specs, no PR API) on three real repos
(retropixel3d-mono: 208 commits/8 branches; emma-pixel-photos; challenge-camerounais):

- **Every derived status spot-checked correct** — including a squash/rebase-merge
  detected via `git cherry`, and `feat/neon-tycoon` correctly marked done while
  work continued on main.
- **Integration-branch election is a required feature, not an edge case:** in 2 of
  3 repos the remote default branch (`origin/HEAD`) was stale/misconfigured; naive
  trust in it produced a 100%-wrong board. Electing by activity among
  integration-named candidates fixed it, with an honest warning surfaced.
- **Solo direct-to-main workflows give branch inference little to bite on** —
  everything is "shadow work" by definition. For solo/agent-heavy users, the
  `Spec: <id>` commit trailer will carry more linking weight than branch globs.
  The V1 design should treat trailers as the primary signal, branches as secondary.
- **The digest already reads well** when commit discipline is good — evidence the
  stakeholder-narrative feature is cheap to make valuable.
- **Multi-developer PR-flow validation (charmbracelet/bubbletea, 1,700+ PRs,
  squash-merge flow):** git-only inference scored **11/11 (100%) on
  done-vs-not-done** against GitHub PR state as ground truth, including a
  squash-merge caught via patch-equivalence. The ≥90% bar (§8.1) is met on
  first contact.
- **The granularity gap is real and bounded:** git alone cannot distinguish
  `in-review` from `in-progress`/`stalled` — that one state requires the forge
  API (`gh`/`glab`), confirming the enrichment-with-graceful-degradation design
  in §6. Notably, git-only "stalled" was often *more honest* than the forge's
  "open": several PRs open on GitHub had seen no commit in 200–800 days.
- **Merged PRs usually delete their branch** (105 of the last 200 on bubbletea) —
  invisible to branch inference, harmless for a live board, but PR/trailer data
  is required for historical digests on PR-flow teams.
- **The drift report doubles as OSS repo housekeeping:** it surfaced 24 zombie
  branches on bubbletea in one command — a possible free-tool wedge for
  distribution ("run it on your repo, see what's rotting").

## 12. Success criteria for V1

- A team of 3+ runs it for 4 weeks with **zero manual status edits** (there is no
  command for it) and the lead reports the weekly digest replaced at least one
  recurring status ritual.
- Status inference ≥90% correct without manual `link` on those repos.
- One unsolicited "can my PM see this?" — proof the stakeholder pull is real.
