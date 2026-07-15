# Truthboard V2 — The Drift Auditor

**Author:** Emmanuel Dadem
**Date:** 2026-07-15
**Status:** Concept — repositioned after validation interviews
**Supersedes:** CONCEPT-V1.md (tracker-replacement positioning; kept for the
object model and prototype findings, which remain valid)

---

## 1. What the interviews said

Verbatim verdict from the tech-lead interviews on the V1 pitch:

> "Would I install this on a team repo this week? **No.** I can't trust that my
> team's messy, real-world git hygiene wouldn't immediately break the status
> inference engine, and my PM would riot if they couldn't access a central web
> UI. **However, I would run it as a local CLI tool on my machine once a week
> to audit my team's sprint.** [...] Consider starting not as a replacement for
> Jira/Linear, but as an **automated auditor** [...] run via a GitHub Action,
> parse the repo, and comment directly on a PR or sync back to Linear. [...]
> The core thesis is absolutely correct."

Decoded:

- **Dead:** "replace your tracker" positioning. Rip-and-replace demands trust
  we haven't earned and takes away the PM's dashboard on day one.
- **Alive and pulled, not pushed:** the **drift report** as an audit tool. The
  interviewee independently described a usage pattern (weekly sprint audit)
  and a distribution channel (GitHub Action + PR comments) we hadn't pitched.
- **The trust objection is the product's job to answer,** not to argue with:
  an auditor that is *read-only* can be adopted at zero risk, and every correct
  drift call it makes earns the trust the V1 tracker would have needed upfront.

## 2. Repositioning

**V1:** Truthboard replaces your tracker; status is derived, never typed.
**V2:** Truthboard is the **auditor of your existing tracker**. It never asks
anyone to update anything and never moves a ticket. It continuously compares
what the tracker *claims* against what the repository *proves*, and reports
the delta — the drift — where people already look (PRs, Slack, a weekly issue).

The core thesis ("the repo is the truth; the board is a claim") is unchanged.
What changed is who owns the board: **we stop trying to own it.** Jira, Linear,
and GitHub Issues keep the dashboard; Truthboard makes it honest.

One sentence: **Linear moves your tickets when PRs merge; Truthboard tells you
when that story stopped being true.**

## 3. The intent source flips (biggest design win)

V1's riskiest assumption (§8.2: devs must adopt a new spec-writing habit)
**dissolves**: in auditor mode, intent already exists — it's the team's
tickets. Adapters read them read-only:

| Intent adapter | Linking signals (already industry convention) |
|---|---|
| Linear | `ENG-123` in branch name / PR title / magic words |
| Jira | `PROJ-123` in branch / commit / PR |
| GitHub Issues | `#123`, `fixes #123` |
| Markdown specs (V1 §5.1) | fallback for repos with no tracker — solo devs, agent-first workflows |

The V1 spec format survives as the *zero-dependency* intent source, not the
required one.

## 4. Product surfaces (in build order)

1. **`truthboard audit` — local CLI.** The prototype, productized: derived
   board + drift report + digest for any repo. Read-only, zero-config, works
   offline. This is the thing the interviewee said they'd run weekly — it is
   the MVP and the demo.
2. **GitHub Action.** Same auditor on a schedule: posts/updates one weekly
   "Drift report" issue (or Slack webhook), and optionally comments on PRs
   with exactly two findings: *"this PR references no ticket"* and *"this PR's
   diff drifts outside ENG-123's declared scope."* Never blocks, never labels,
   never closes — auditors that nag get uninstalled.
3. **Tracker adapters (Linear first).** Enrich the audit with claim-vs-proof
   findings impossible for git alone: ticket says In Progress with zero
   commits for 9 days; ticket says Done but the merge was reverted; PR merged
   but Linear's own automation failed to move the ticket (their sync has
   documented failure modes with multi-PR issues and unstable checks — we
   audit the auditomation).
4. **Deferred:** sync-*back* (writing statuses into Linear/Jira). Linear
   already moves tickets on PR events; our write-back adds little until the
   audit has earned trust. Revisit only if users ask. The V1 full tracker
   remains the possible long game.

## 5. What "derived, never typed" means now

The principle survives intact in auditor form: Truthboard **never asks a human
for input at all** — no specs required, no status edits possible, nothing to
maintain. Its entire output is the computed delta between two things other
people already maintain (the tracker and the repo). The honesty principle
(V1 §4.4) also carries: uncertain inference is reported as "unverifiable,"
never guessed — an auditor's false alarm costs more than a missed one.

## 6. Competitive position (checked 2026-07-15)

- **Stale-branch / stale-issue Actions** (actions/stale, remove-stale-branches):
  janitors, not auditors — they delete/close by age, with no notion of intent
  or claim-vs-proof.
- **Linear / Jira git integrations:** they *move* tickets on PR events (one
  direction: git event → status change) but never verify the resulting board
  against the repo, and their automations have documented silent-failure
  modes. They are an input pipe; we are the audit.
- **LinearB / Swarmia / engineering-intelligence platforms:** manager-facing
  metrics dashboards (DORA, cycle time), SaaS, top-down purchase. Truthboard
  is repo-facing, finding-oriented, bottom-up, and free to run locally.
- **Nobody** answers "where is the board lying?" as a product.

## 7. MVP cut (V2)

**In:** Go single binary `truthboard audit` (port of the validated prototype:
integration-branch election, squash-merge detection via patch-equivalence,
stalled/shadow/zombie findings, digest); GitHub Issues adapter (no extra auth
needed in Actions context); the GitHub Action wrapper (schedule → one
updateable drift issue); `--format md|json`.

**Out (V2.1+):** Linear/Jira adapters, PR comments, Slack webhook, scope-creep
detection (needs per-ticket path hints), the V1 spec format, any web UI.

**Success criteria:**
- One team (not Emmanuel's own repo) keeps the weekly Action installed for 4
  consecutive weeks — retention, not installs, is the metric.
- A drift finding causes a real correction (ticket closed, branch deleted,
  zombie work resurrected) at least once a week per active repo.
- Zero findings disputed as "the auditor is wrong" in those 4 weeks — the
  trust bar. A disputed finding is a P0 bug.

## 8. Risks carried forward

1. **"Messy git hygiene breaks the inference"** — now a feature, not a bug:
   messy hygiene *is drift*, and the auditor reports it as such. But findings
   must be phrased as questions about the work, not accusations about the
   person (V1 §8.4 surveillance risk applies double in PR comments — public,
   permanent, name-attached).
2. **Noise kills auditors.** Default output must be scannable in 30 seconds;
   every finding class needs a per-repo mute. Better to under-report in v1.
3. **Forge coupling.** Starting GitHub-only contradicts the local-first créd;
   acceptable because the local CLI stays forge-agnostic (pure git), and the
   Action is just a scheduler around it.
