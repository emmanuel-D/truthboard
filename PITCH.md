# Truthboard

## Your repo already knows the status. Stop typing it twice.

**The problem.** Your tracker is a database of claims that developers must keep
in sync with reality by hand. Nobody does, so the board rots — and then the
compensation rituals begin: standups that are really interrogations, "quick
status" pings, story-point theater, and a weekly report someone writes by
re-reading git anyway. Meanwhile the actual truth — branches, commits, PRs, CI,
AI-agent sessions — sits in the repo, perfectly machine-readable, ignored.

**The product.** Truthboard is a local-first, git-native delivery tracker with
one rule: **status is derived, never typed.** There is no command to move a
card. Humans (or their AI agents) write down *intent* once — a 10-line markdown
spec, the same brief you'd hand Claude Code anyway — and everything else is
computed from repository facts:

- **Board** — spec + matching branch → `in-progress`; PR open → `in-review`;
  merged + CI green → `done`; no activity for a week → `stalled`; reverted →
  `regressed`. Wrong status? You fix the *inputs* (link the branch), never the
  output.
- **Drift report** — where is the board lying? Shadow work (commits nobody
  promised), stale promises (specs nobody's working), scope creep (diffs far
  outside the spec), regressions. The board earns trust; the standup stops
  being a status meeting.
- **Digest** — a readable "what shipped and why" narrative for stakeholders,
  generated from intent + merges. The status report writes itself.
- **Agent-native** — every spec doubles as an AI agent's context packet, and
  agent sessions attach to the spec they executed: *spec → diff → review →
  outcome*, finally auditable.

**Why believe the inference works.** A prototype scanner ran on four real
repos (solo GitLab projects + charmbracelet/bubbletea with 1,700+ squash-merged
PRs): **100% correct on done-vs-not-done** against PR state as ground truth,
including squash merges and a misconfigured remote default branch it detected
and corrected. Where git alone can't see a state (in-review), Truthboard says
so honestly instead of guessing.

**What it is not.** No server, no accounts, no workflow configuration, no story
points, no velocity charts. One static binary; plain markdown files in your
repo; works offline; your data never leaves git.

|  | Jira / Linear | Backlog.md | Truthboard |
|---|---|---|---|
| Humans enter | everything, forever | tasks + status edits | a 10-line spec, once |
| Status source | humans drag cards | humans edit files | **derived from git** |
| Trustworthy? | rots immediately | rots politely | can't rot — it *is* the repo |

---

*Emmanuel Dadem · concept stage · seeking 15 minutes of brutal honesty:*
*Would you install this on your team's repo this week? If not — what's the*
*first thing that would stop you?*
