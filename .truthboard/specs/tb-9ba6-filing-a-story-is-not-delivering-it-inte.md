---
id: tb-9ba6
title: 'Filing a story is not delivering it: intent-only commits stop deriving done'
owner: emmanuel
branch: '*/tb-9ba6-*'
paths:
    - internal/audit/**
    - README.md
epic: po-experience
priority: 1
type: bug
---

## Goal

As anyone filing a story, I want writing it down to leave it in the
backlog, not on the "delivered" pile. Today it cannot: committing a new
spec file to the integration branch with its own `Spec: <id>` trailer —
the documented way to add intent, and the pattern in this repo's own
history — makes that spec derive **done** the moment it is pushed. The
story appears in the digest as "✓ … landed", `next_spec` will never hand
it to an idle agent, and the backlog silently loses an item at the exact
moment someone tried to add one.

Observed on tb-2c89, filed 2026-08-11: one commit, one file
(`.truthboard/specs/tb-2c89-*.md`), zero implementation, status DONE with
evidence "work landed on origin/main", and a "✓ … landed 2026-08-11" line
in the digest.

There is no way to avoid it today, which is what makes this a defect and
not a habit to correct. The three available moves are: land intent on the
integration branch (reads as **done**), put it on a branch (reads as
**in-progress** — a story nobody has started), or omit the trailer (reads
as **shadow work** in the audit). Every route lies. Meanwhile the tool
already knows better in another corner of the same file: `governedFile`
(`internal/audit/audit.go`) exempts commits touching only `.truthboard/**`
and the wiring files from shadow work, with the comment "writing a story
is intent, not work — backlog grooming ... land directly on the
integration branch by design". `landingCommit` (`internal/audit/specs.go`)
simply doesn't consult it.

**The fix is to reuse that predicate, not invent a second one.** A commit
whose entire file list is governed is intent; it must not be elected as a
landing commit. A commit that touches a spec *and* something else is
ordinary work — the spec edit rides along with the implementation, which
is how most stories here actually land, and that must keep deriving done.
When every trailered commit for a spec is intent-only, the spec stays
`planned` — which is the truth, and which puts it back in `next_spec`'s
queue.

Acceptance checkboxes must **not** be load-bearing here. Plenty of
genuinely delivered specs in this repo carry 0 ticked boxes (tb-a4ab,
tb-fbb0, tb-eeb7 and others), so "unticked ⇒ not done" would regress
dozens of true statuses. Ticking stays what it is: a human signal, never
the derivation. This also stays inside the product rule — nothing is
typed, nothing gains a status field; the derivation just stops reading a
file-creation as a delivery.

## Acceptance

- [x] **Given** a commit that touches only files `governedFile` accepts and
  carries `Spec: <id>`, **when** it lands on the integration branch,
  **then** it is not elected as that spec's landing commit and the spec
  stays `planned`
- [x] **Given** the same commit, **then** it is still exempt from shadow
  work — filing a story must not become the other kind of lie, and the two
  behaviours must be driven by the *same* predicate, not two copies that
  can drift apart
- [x] **Given** a commit that edits a spec file *and* source files, **then**
  it lands the spec exactly as it does today: the common case of the
  implementation carrying its own acceptance ticks must not regress
- [x] **Given** a spec with several trailered commits where only some are
  intent-only, **then** the newest non-intent commit is the landing
  commit, and its SHA is what evidence and CI checks are read against
- [x] **Given** a spec whose acceptance boxes are entirely unticked but
  whose implementation landed, **then** it still derives `done` — ticking
  is a human signal and never gates the derivation (regression guard for
  tb-a4ab, tb-fbb0, tb-eeb7 and every other 0-of-N done spec)
- [x] **Given** the digest, **then** a story whose only landed commit is
  its own intent no longer appears in `Shipped` / "✓ … landed"
- [x] **Given** `next_spec` and an idle agent, **then** a story filed this
  way is handed out as startable — the harm this bug does to the agent
  loop is the one that has to be proven fixed
- [x] **Given** tb-2c89 specifically, **then** re-running the audit on the
  current history reports it `planned`, with no history rewrite required
- [x] The README's linking-signals paragraph states that a commit touching
  only intent files links a story without landing it, so the rule is
  documented where the three signals are already listed
