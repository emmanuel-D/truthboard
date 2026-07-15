#!/usr/bin/env python3
"""Truthboard prototype scanner.

Derives work-unit statuses and a drift report from git history alone —
no specs, no PR API, no manual input. Each non-integration branch is
treated as an implicit unit of work. Tests the core bet of CONCEPT-V1.md:
can status be inferred from repo reality with ~90% accuracy?

Usage: scan.py /path/to/repo [--stale-days 7] [--digest-days 14]
"""

import argparse
import datetime
import json
import re
import subprocess
import sys
from dataclasses import dataclass, field

INTEGRATION = {"main", "master", "develop", "release", "trunk"}
MR_MERGE_RE = re.compile(r"See merge request|Merge branch|Merge pull request", re.I)

C = {"green": "\033[32m", "yellow": "\033[33m", "red": "\033[31m",
     "cyan": "\033[36m", "dim": "\033[2m", "bold": "\033[1m", "off": "\033[0m"}


def paint(txt, color):
    return f"{C[color]}{txt}{C['off']}" if sys.stdout.isatty() else txt


def git(repo, *args, check=True):
    r = subprocess.run(["git", "-C", repo, *args], capture_output=True, text=True)
    if check and r.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)}: {r.stderr.strip()}")
    return r.stdout.strip()


def try_git(repo, *args):
    r = subprocess.run(["git", "-C", repo, *args], capture_output=True, text=True)
    return r.returncode == 0, r.stdout.strip()


@dataclass
class Unit:
    name: str
    tip: str
    last_commit: datetime.datetime
    status: str = "unknown"
    evidence: str = ""
    ahead: int = 0
    behind: int = 0
    flags: list = field(default_factory=list)


def default_branch(repo, branches):
    """Elect the integration branch. origin/HEAD is the first hint, but a
    stale/misconfigured remote default must not poison every inference, so
    among integration-named candidates the most recently active tip wins."""
    hint = None
    ok, ref = try_git(repo, "symbolic-ref", "refs/remotes/origin/HEAD")
    if ok:
        hint = ref.rsplit("/", 1)[-1]
    candidates = {n: w for n, (_, w) in branches.items()
                  if n in INTEGRATION or n == hint}
    if not candidates:
        raise RuntimeError("cannot determine integration branch")
    elected = max(candidates, key=candidates.get)
    note = None
    if hint and elected != hint:
        note = (f"origin/HEAD points to '{hint}' (last active "
                f"{candidates.get(hint, branches.get(hint, (None, None))[1]):%Y-%m-%d}) but '{elected}' is newer — "
                f"remote default branch looks misconfigured")
    return elected, ("origin/HEAD" if elected == hint else "activity election"), note


def collect_branches(repo):
    """Local + origin branches, dedup by short name, prefer newest tip."""
    out = git(repo, "for-each-ref", "refs/heads", "refs/remotes/origin",
              "--format=%(refname)|%(objectname)|%(committerdate:iso8601-strict)")
    seen = {}
    for line in out.splitlines():
        ref, sha, date = line.split("|")
        if ref.endswith("/HEAD"):
            continue
        name = ref.removeprefix("refs/heads/").removeprefix("refs/remotes/origin/")
        when = datetime.datetime.fromisoformat(date.replace("Z", "+00:00"))
        if name not in seen or when > seen[name][1]:
            seen[name] = (sha, when)
    return {n: v for n, v in seen.items()}


def integration_ref(repo, default):
    ok, _ = try_git(repo, "rev-parse", "--verify", f"origin/{default}")
    return f"origin/{default}" if ok else default


def classify(repo, base, name, sha, when, now, stale_days):
    u = Unit(name=name, tip=sha, last_commit=when)
    ok, _ = try_git(repo, "merge-base", "--is-ancestor", sha, base)
    if ok:
        u.status, u.evidence = "done", f"tip is ancestor of {base} (merged)"
        return u
    # squash/rebase merge detection: git cherry marks patch-equivalent commits with '-'
    ok, cherry = try_git(repo, "cherry", base, sha)
    if ok and cherry:
        marks = [l[0] for l in cherry.splitlines() if l]
        if all(m == "-" for m in marks):
            u.status, u.evidence = "done", f"all {len(marks)} commits patch-equivalent in {base} (squash/rebase merge)"
            return u
        if any(m == "-" for m in marks):
            u.flags.append(f"{marks.count('-')}/{len(marks)} commits already in {base} (partial merge)")
    ok, counts = try_git(repo, "rev-list", "--left-right", "--count", f"{base}...{sha}")
    if ok:
        behind, ahead = (int(x) for x in counts.split())
        u.ahead, u.behind = ahead, behind
    age = (now - when).days
    if age > stale_days:
        u.status, u.evidence = "stalled", f"no commits for {age} days ({u.ahead} unmerged)"
    else:
        u.status, u.evidence = "in-progress", f"active {age}d ago, {u.ahead} commits ahead, {u.behind} behind"
    return u


def shadow_work(repo, base, days):
    """Non-merge commits landing directly on the integration branch that
    don't look like an MR/PR squash merge — work that bypassed review."""
    out = git(repo, "log", base, "--first-parent", "--no-merges",
              f"--since={days}.days", "--format=%h|%cs|%an|%s")
    rows = []
    for line in out.splitlines():
        h, date, author, subj = line.split("|", 3)
        if not MR_MERGE_RE.search(subj):
            rows.append((h, date, author, subj))
    return rows


def digest(repo, base, days):
    out = git(repo, "log", base, "--first-parent", f"--since={days}.days",
              "--format=%h|%cs|%s")
    return [tuple(l.split("|", 2)) for l in out.splitlines()]


def github_prs(repo, limit=200):
    r = subprocess.run(["gh", "pr", "list", "--state", "all", "--limit", str(limit),
                        "--json", "number,title,state,isDraft,headRefName,updatedAt,mergedAt"],
                       cwd=repo, capture_output=True, text=True)
    return json.loads(r.stdout) if r.returncode == 0 else None


def validate_against_prs(units, prs):
    """Score git-only inference against PR state as ground truth.

    The binary question git-only inference must answer correctly is
    done vs not-done; review/draft granularity is only visible to the
    forge API and is reported as a gap, not an error."""
    by_head = {}
    for p in prs:  # gh returns newest-first; keep the newest PR per branch
        by_head.setdefault(p["headRefName"], p)
    rows, agree = [], 0
    for u in units:
        p = by_head.get(u.name)
        if not p:
            continue
        truth = ("done" if p["state"] == "MERGED"
                 else "abandoned" if p["state"] == "CLOSED"
                 else "in-progress (draft)" if p["isDraft"] else "in-review")
        ok = (u.status == "done") == (p["state"] == "MERGED")
        agree += ok
        rows.append((u.name, u.status, f"#{p['number']} {truth}", ok))
    merged_deleted = sum(1 for p in prs if p["state"] == "MERGED"
                         and not any(u.name == p["headRefName"] for u in units))
    return rows, agree, merged_deleted


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo")
    ap.add_argument("--stale-days", type=int, default=7)
    ap.add_argument("--digest-days", type=int, default=14)
    ap.add_argument("--github-validate", action="store_true",
                    help="score git-only inference against GitHub PR state via gh")
    args = ap.parse_args()
    repo, now = args.repo, datetime.datetime.now().astimezone()

    branches = collect_branches(repo)
    default, how, note = default_branch(repo, branches)
    base = integration_ref(repo, default)
    print(f"\n{paint('TRUTHBOARD SCAN', 'bold')}  {repo}")
    print(f"integration branch: {paint(base, 'cyan')} (via {how})")
    if note:
        print(paint(f"⚠ {note}", "yellow"))
    print()
    units = []
    for name, (sha, when) in sorted(branches.items()):
        if name in INTEGRATION or name == default:
            continue
        units.append(classify(repo, base, name, sha, when, now, args.stale_days))

    # ---- board ----
    print(paint("DERIVED BOARD (no human ever set these statuses)", "bold"))
    color = {"done": "green", "in-progress": "cyan", "stalled": "yellow", "unknown": "red"}
    width = max((len(u.name) for u in units), default=10) + 2
    for st in ("in-progress", "stalled", "done", "unknown"):
        for u in (x for x in units if x.status == st):
            print(f"  {paint(st.upper().ljust(12), color[st])} {u.name.ljust(width)} "
                  f"{C['dim'] if sys.stdout.isatty() else ''}{u.evidence}{C['off'] if sys.stdout.isatty() else ''}")
            for f in u.flags:
                print(f"  {' ' * 12} {' ' * width} {paint('⚠ ' + f, 'yellow')}")
    if not units:
        print("  (no work-unit branches found)")

    # ---- github validation ----
    if args.github_validate:
        prs = github_prs(repo)
        if prs is None:
            print(f"\n{paint('VALIDATION: gh unavailable for this repo', 'yellow')}")
        else:
            rows, agree, merged_deleted = validate_against_prs(units, prs)
            print(f"\n{paint('VALIDATION — git-only inference vs GitHub PR truth', 'bold')}")
            for name, inferred, truth, ok in rows:
                mark = paint("✓", "green") if ok else paint("✗ MISMATCH", "red")
                print(f"  {mark} {name[:44].ljust(46)} git-only: {inferred.ljust(12)} truth: {truth}")
            if rows:
                pct = 100 * agree / len(rows)
                verdict = "green" if pct >= 90 else "red"
                print(paint(f"  score: {agree}/{len(rows)} ({pct:.0f}%) on done-vs-not-done "
                            f"[bar from CONCEPT-V1 §8: ≥90%]", verdict))
            granular = sum(1 for _, inf, tr, _ in rows
                           if "in-review" in tr and inf in ("in-progress", "stalled"))
            if granular:
                print(f"  granularity gap: {granular} branches are in-review per GitHub; "
                      f"git alone can only see in-progress/stalled")
            if merged_deleted:
                print(f"  note: {merged_deleted} merged PRs had their branch deleted — "
                      f"invisible to branch inference, fine for a live board")

    # ---- drift report ----
    print(f"\n{paint('DRIFT REPORT', 'bold')}")
    stale = [u for u in units if u.status == "stalled"]
    if stale:
        print(paint(f"  Stale promises ({len(stale)}): work that stopped without landing", "yellow"))
        for u in stale:
            print(f"    - {u.name}: {u.evidence}")
    merged_undeleted = [u for u in units if u.status == "done"]
    if merged_undeleted:
        print(paint(f"  Landed but branch not deleted ({len(merged_undeleted)}):", "dim"))
        for u in merged_undeleted:
            print(f"    - {u.name}")
    shadows = shadow_work(repo, base, args.digest_days)
    if shadows:
        print(paint(f"  Shadow work ({len(shadows)}): commits on {base} outside any branch/MR flow "
                    f"(last {args.digest_days}d)", "red"))
        for h, date, author, subj in shadows[:15]:
            print(f"    - {date} {h} {author}: {subj[:70]}")
        if len(shadows) > 15:
            print(f"      … and {len(shadows) - 15} more")
    if not (stale or shadows):
        print(paint("  clean — board matches reality", "green"))

    # ---- digest ----
    print(f"\n{paint(f'DIGEST — what landed on {base} in the last {args.digest_days} days', 'bold')}")
    landed = digest(repo, base, args.digest_days)
    for h, date, subj in landed[:20]:
        print(f"  {date} {subj[:80]}")
    if not landed:
        print("  nothing landed")
    print()


if __name__ == "__main__":
    main()
