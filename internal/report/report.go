// Package report renders an audit.Result for humans (terminal), for the
// weekly drift issue (markdown), and for automation (json).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

var statusOrder = []audit.Status{audit.InReview, audit.InProgress, audit.Stalled, audit.Done}

// Regressed leads: a done that came undone is the loudest thing a board can say.
var specStatusOrder = []audit.Status{audit.Regressed, audit.InReview, audit.InProgress, audit.Planned, audit.Stalled, audit.Done}

// forgeLabel joins every forge that answered — the hub's and each enriched
// spoke's. Per-spoke enrichment means claims can exist even when the hub
// itself has no forge, so the claims sections key off this, not res.Forge.
func forgeLabel(res *audit.Result) string {
	var forges []string
	if res.Forge != "" {
		forges = append(forges, res.Forge)
	}
	for _, r := range res.Workspace {
		if r.Forge != "" {
			forges = append(forges, r.Forge)
		}
	}
	return strings.Join(forges, ", ")
}

var ansi = map[audit.Status]string{
	audit.Regressed:  "\033[31m",
	audit.InReview:   "\033[35m",
	audit.InProgress: "\033[36m",
	audit.Planned:    "\033[34m",
	audit.Stalled:    "\033[33m",
	audit.Done:       "\033[32m",
}

var claimHeadlines = map[string]string{
	"ticket-done-but-open": "Tickets already done but still open",
	"ticket-stale":         "Open tickets with no repo activity",
	"unticketed-work":      "Work nobody promised (no ticket, no PR)",
	"pr-abandoned":         "PRs closed without merging, branch still alive",
}

var claimOrder = []string{"ticket-done-but-open", "ticket-stale", "unticketed-work", "pr-abandoned"}

// claimCap limits findings shown per kind; noise gets auditors uninstalled
// (CONCEPT-V2 §8.2), and the JSON format always carries the full list.
const claimCap = 10

// backlogTag renders " · p1 · epic-name" for whatever backlog intent the
// spec declares; empty when it declares none.
func backlogTag(s audit.SpecStatus) string {
	var parts []string
	if s.Priority > 0 {
		parts = append(parts, fmt.Sprintf("p%d", s.Priority))
	}
	if s.Epic != "" {
		parts = append(parts, s.Epic)
	}
	if s.Sprint != "" {
		parts = append(parts, s.Sprint)
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

// commitTag renders "api: " in front of a commit that landed in a
// workspace spoke; hub commits stay unprefixed.
func commitTag(cm audit.Commit) string {
	if cm.Repo == "" {
		return ""
	}
	return cm.Repo + ": "
}

// flowUnmeasurableCap is how many untimeable stories the terminal names
// before it summarises the rest — a repo adopting the trailer late has a
// backlog of them, and the count is the point.
const flowUnmeasurableCap = 5

var sparkBlocks = []rune("▁▂▃▄▅▆▇█")

// sparkline draws weekly throughput as one line, oldest week to newest. A
// week nothing landed in is drawn as a gap rather than a low bar: a stall
// and a slow week are different facts, and a smoothed curve through the
// middle of them would be neither.
func sparkline(f *audit.Flow) string {
	weeks := f.Weeks
	if len(weeks) > 26 { // half a year at a glance; older weeks are in the JSON
		weeks = weeks[len(weeks)-26:]
	}
	peak := 0
	for _, b := range weeks {
		if b.Stories > peak {
			peak = b.Stories
		}
	}
	if peak == 0 {
		return ""
	}
	var line strings.Builder
	for _, b := range weeks {
		if b.Stories == 0 {
			line.WriteRune('·')
			continue
		}
		i := (b.Stories*len(sparkBlocks) - 1) / peak
		line.WriteRune(sparkBlocks[i])
	}
	return fmt.Sprintf("%s %s %s  peak %d", weeks[0].Start, line.String(), weeks[len(weeks)-1].End, peak)
}

// sprintPoints renders " · 5/13 pts (2 unestimated)" when the sprint has
// estimated stories; empty otherwise so point-free repos see no change.
func sprintPoints(sp audit.SprintRollup) string {
	if sp.PointsTotal == 0 {
		return ""
	}
	out := fmt.Sprintf(" · %d/%d pts", sp.PointsDone, sp.PointsTotal)
	if sp.Unestimated > 0 {
		out += fmt.Sprintf(" (%d unestimated)", sp.Unestimated)
	}
	return out
}

// sprintWindow renders the derived calendar state for a dated sprint:
// " · 2026-07-14 → 2026-07-25 · active, 8d left". Empty for date-less
// sprints, which keep their original arithmetic-only line.
func sprintWindow(sp audit.SprintRollup) string {
	if sp.State == "" {
		return ""
	}
	out := fmt.Sprintf(" · %s → %s · %s", sp.Start, sp.End, sp.State)
	if sp.State == "active" {
		if sp.DaysLeft == 0 {
			out += ", ends today"
		} else {
			out += fmt.Sprintf(", %dd left", sp.DaysLeft)
		}
	}
	return out
}

// sprintDetail is everything after the done count: the drift-prone half,
// and the reason this bug existed. Terminal rendered it and Markdown did
// not, so the format most likely to reach a non-developer was the one
// missing the numbers they needed. Any third surface composes from here.
func sprintDetail(sp audit.SprintRollup) string {
	return sprintPoints(sp) + sprintWindow(sp)
}

// sprintFacts is the whole line body — what every renderer must say about a
// sprint, however it decorates it.
func sprintFacts(sp audit.SprintRollup) string {
	return fmt.Sprintf("%d/%d done", sp.Done, sp.Total) + sprintDetail(sp)
}

func countClaims(claims []audit.Claim, kind string) int {
	n := 0
	for _, c := range claims {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

const (
	ansiOff    = "\033[0m"
	ansiBold   = "\033[1m"
	ansiDim    = "\033[2m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiCyan   = "\033[36m"
)

func JSON(w io.Writer, res *audit.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}

func Terminal(w io.Writer, res *audit.Result, color bool) error {
	c := func(code, s string) string {
		if !color {
			return s
		}
		return code + s + ansiOff
	}

	fmt.Fprintf(w, "\n%s  %s\n", c(ansiBold, "TRUTHBOARD AUDIT"), res.Repo)
	fmt.Fprintf(w, "integration branch: %s (via %s)\n", c(ansiCyan, res.Integration), res.ElectedVia)
	if res.ElectionNote != "" {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, "⚠ "+res.ElectionNote))
	}
	if len(res.Workspace) > 0 {
		fmt.Fprintf(w, "workspace:")
		for _, r := range res.Workspace {
			if r.Err == "" {
				fmt.Fprintf(w, " %s (%s)", c(ansiCyan, r.Name), r.Integration)
			} else {
				fmt.Fprintf(w, " %s", c(ansiRed, r.Name+" ✗"))
			}
		}
		fmt.Fprintln(w)
		for _, r := range res.Workspace {
			if r.Err != "" {
				fmt.Fprintf(w, "%s\n", c(ansiRed, "⚠ "+r.Name+": "+r.Err))
			} else if r.ForgeNote != "" {
				fmt.Fprintf(w, "%s\n", c(ansiYellow, "◦ "+r.Name+": "+r.ForgeNote))
			}
		}
	}

	if len(res.Specs) > 0 {
		fmt.Fprintf(w, "\n%s\n", c(ansiBold, "SPEC BOARD (intent from .truthboard/specs — status derived, never typed)"))
		idWidth := 6
		for _, s := range res.Specs {
			if len(s.ID) > idWidth {
				idWidth = len(s.ID)
			}
		}
		for _, st := range specStatusOrder {
			for _, s := range res.Specs {
				if s.Status != st {
					continue
				}
				branches := ""
				if len(s.Branches) > 0 {
					branches = " [" + strings.Join(s.Branches, ", ") + "]"
				}
				fmt.Fprintf(w, "  %s %-*s %s%s%s\n    %s\n",
					c(ansi[st], fmt.Sprintf("%-12s", strings.ToUpper(string(st)))),
					idWidth, s.ID, s.Title, c(ansiDim, backlogTag(s)), branches, c(ansiDim, s.Evidence))
			}
		}
	}

	if len(res.Sprints) > 0 {
		fmt.Fprintf(w, "\n%s\n", c(ansiBold, "SPRINTS (arithmetic over derived statuses — a sprint finishes when its stories land)"))
		for _, sp := range res.Sprints {
			fmt.Fprintf(w, "  %s  %d/%d done%s\n", c(ansiCyan, sp.Name), sp.Done, sp.Total, c(ansiDim, sprintDetail(sp)))
			for _, o := range sp.Open {
				fmt.Fprintf(w, "    %s %s %s\n",
					c(ansi[o.Status], fmt.Sprintf("%-12s", strings.ToUpper(string(o.Status)))), o.ID, o.Title)
			}
		}
	}

	if f := res.Flow; f.Measured() {
		fmt.Fprintf(w, "\n%s\n", c(ansiBold, "FLOW (timed from commits — first work commit to the merge that landed it)"))
		fmt.Fprintf(w, "  %s\n", f.Headline)
		if len(f.Weeks) > 0 {
			fmt.Fprintf(w, "  %s %s\n", c(ansiDim, "landed/week"), sparkline(f))
		}
		if len(f.Unmeasurable) > 0 {
			fmt.Fprintf(w, "  %s\n", c(ansiYellow, fmt.Sprintf("  not timeable (%d): landed, but the history cannot say how long it took", len(f.Unmeasurable))))
			for i, u := range f.Unmeasurable {
				if i == flowUnmeasurableCap {
					fmt.Fprintf(w, "    %s\n", c(ansiDim, fmt.Sprintf("… and %d more", len(f.Unmeasurable)-flowUnmeasurableCap)))
					break
				}
				fmt.Fprintf(w, "    - %s %s — %s\n", u.ID, truncate(u.Title, 40), u.Reason)
			}
		}
	}

	fmt.Fprintf(w, "\n%s\n", c(ansiBold, "DERIVED BOARD (no human ever set these statuses)"))
	width := 10
	for _, u := range res.Units {
		if len(u.Label()) > width {
			width = len(u.Label())
		}
	}
	width += 2
	shown := 0
	for _, st := range statusOrder {
		for _, u := range res.Units {
			if u.Status != st {
				continue
			}
			shown++
			fmt.Fprintf(w, "  %s %-*s %s\n",
				c(ansi[st], fmt.Sprintf("%-12s", strings.ToUpper(string(st)))),
				width, u.Label(), c(ansiDim, u.Evidence))
			for _, f := range u.Flags {
				fmt.Fprintf(w, "  %12s %-*s %s\n", "", width, "", c(ansiYellow, "⚠ "+f))
			}
		}
	}
	if shown == 0 {
		fmt.Fprintln(w, "  (no work-unit branches found)")
	}

	fmt.Fprintf(w, "\n%s\n", c(ansiBold, "DRIFT REPORT"))
	d := res.Drift
	if len(d.StalePromises) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Stale promises (%d): work that stopped without landing", len(d.StalePromises))))
		for _, u := range d.StalePromises {
			fmt.Fprintf(w, "    - %s: %s\n", u.Label(), u.Evidence)
		}
	}
	if len(d.LandedNotDeleted) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiDim, fmt.Sprintf("  Landed but branch not deleted (%d):", len(d.LandedNotDeleted))))
		for _, name := range d.LandedNotDeleted {
			fmt.Fprintf(w, "    - %s\n", name)
		}
	}
	if len(d.ShadowWork) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiRed, fmt.Sprintf("  Shadow work (%d): commits on %s outside any branch/MR flow (last %dd)",
			len(d.ShadowWork), res.Integration, res.DigestDays)))
		for i, cm := range d.ShadowWork {
			if i == 15 {
				fmt.Fprintf(w, "      … and %d more\n", len(d.ShadowWork)-15)
				break
			}
			fmt.Fprintf(w, "    - %s%s %s %s: %s\n", commitTag(cm), cm.Date, cm.Hash, cm.Author, truncate(cm.Subject, 70))
		}
	}
	if len(d.DependencyCycles) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiRed, fmt.Sprintf("  Dependency cycles (%d): intent that can never become ready", len(d.DependencyCycles))))
		for _, cy := range d.DependencyCycles {
			fmt.Fprintf(w, "    - %s\n", cy)
		}
	}
	if len(d.UnknownRepos) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiRed, fmt.Sprintf("  Unknown repos (%d): repos: intent naming repos the workspace does not declare", len(d.UnknownRepos))))
		for _, ur := range d.UnknownRepos {
			fmt.Fprintf(w, "    - %s\n", ur)
		}
	}
	if len(d.UnwiredRepos) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Unwired spokes (%d): checked out and watched for proof, but agents there have no board", len(d.UnwiredRepos))))
		for _, ur := range d.UnwiredRepos {
			fmt.Fprintf(w, "    - %s\n", ur)
		}
	}
	if len(d.ScopeCreep) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Scope creep (%d): linked work drifting outside declared spec paths", len(d.ScopeCreep))))
		for _, sc := range d.ScopeCreep {
			fmt.Fprintf(w, "    - %s / %s: %d%% of the diff (%d/%d files) outside spec paths — mostly %s\n",
				sc.SpecID, sc.Branch, 100*sc.Outside/sc.Total, sc.Outside, sc.Total, sc.TopDirs)
		}
	}
	if len(d.ContradictedHolds) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Contradicted holds (%d): a paused reason the evidence disagrees with", len(d.ContradictedHolds))))
		for _, h := range d.ContradictedHolds {
			fmt.Fprintf(w, "    - %s %s — held for %q, but %s\n", h.ID, truncate(h.Title, 46), h.Hold, h.Why)
		}
	}
	if len(d.UnverifiedAcceptance) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Unverified acceptance (%d): landed work whose criteria were never ticked", len(d.UnverifiedAcceptance))))
		for i, ua := range d.UnverifiedAcceptance {
			// A repo adopting this signal late has a backlog of them; the
			// count is the point, the full list is what audit --format md is for.
			if i == 10 {
				fmt.Fprintf(w, "    %s\n", c(ansiDim, fmt.Sprintf("… and %d more — truthboard check <id> as you verify each", len(d.UnverifiedAcceptance)-10)))
				break
			}
			fmt.Fprintf(w, "    - %s %s — %s\n", ua.ID, truncate(ua.Title, 46), ua.Summary())
		}
	}
	if d.Clean() {
		fmt.Fprintf(w, "%s\n", c(ansiGreen, "  clean — board matches reality"))
	}

	if forges := forgeLabel(res); forges != "" {
		fmt.Fprintf(w, "\n%s\n", c(ansiBold, fmt.Sprintf("CLAIMS vs PROOF — tracker: %s", forges)))
		if len(res.Claims) == 0 {
			fmt.Fprintf(w, "%s\n", c(ansiGreen, "  clean — every tracker claim is backed by the repo"))
		}
		for _, kind := range claimOrder {
			shown := 0
			for _, cl := range res.Claims {
				if cl.Kind != kind {
					continue
				}
				if shown == 0 {
					fmt.Fprintf(w, "%s\n", c(ansiYellow, "  "+claimHeadlines[kind]+":"))
				}
				if shown == claimCap {
					fmt.Fprintf(w, "      … and %d more\n", countClaims(res.Claims, kind)-claimCap)
					break
				}
				fmt.Fprintf(w, "    - %s: %s\n", cl.Subject, cl.Detail)
				shown++
			}
		}
	}

	fmt.Fprintf(w, "\n%s\n", c(ansiBold, fmt.Sprintf("DIGEST — what landed on %s in the last %d days", res.Integration, res.DigestDays)))
	line := func(sh audit.ShippedSpec) {
		tag := ""
		if sh.Epic != "" {
			tag = " · " + sh.Epic
		}
		fmt.Fprintf(w, "  %s %s (%s%s)\n", c(ansiGreen, "✓ "+sh.Title), c(ansiDim, "landed "+sh.Date), sh.ID, tag)
	}
	// One type of work → the familiar flat list. Mixed types → grouped, so
	// a release note can separate features from fixes at a glance.
	typeOf := func(sh audit.ShippedSpec) string {
		if sh.Type == "" {
			return "story"
		}
		return sh.Type
	}
	distinct := map[string]bool{}
	for _, sh := range res.Shipped {
		distinct[typeOf(sh)] = true
	}
	if len(distinct) > 1 {
		for _, g := range [][2]string{{"story", "Features"}, {"bug", "Fixes"}, {"task", "Chores"}} {
			if !distinct[g[0]] {
				continue
			}
			fmt.Fprintf(w, "  %s\n", c(ansiDim, g[1]+":"))
			for _, sh := range res.Shipped {
				if typeOf(sh) == g[0] {
					line(sh)
				}
			}
		}
	} else {
		for _, sh := range res.Shipped {
			line(sh)
		}
	}
	other := 0
	for _, cm := range res.Digest {
		if cm.Spec == "" {
			other++
		}
	}
	if other > 0 && len(res.Shipped) > 0 {
		fmt.Fprintf(w, "  %s\n", c(ansiDim, "also landed:"))
	}
	shown = 0
	for _, cm := range res.Digest {
		if cm.Spec != "" {
			continue
		}
		if shown == 20 {
			fmt.Fprintf(w, "  … and %d more\n", other-20)
			break
		}
		fmt.Fprintf(w, "  %s %s%s\n", cm.Date, commitTag(cm), truncate(cm.Subject, 80))
		shown++
	}
	if len(res.Digest) == 0 {
		fmt.Fprintln(w, "  nothing landed")
	}
	fmt.Fprintln(w)
	return nil
}

func Markdown(w io.Writer, res *audit.Result) error {
	repoLabel := res.Repo
	if res.Forge != "" {
		repoLabel = res.Forge
	}
	fmt.Fprintf(w, "## Truthboard drift report\n\n")
	fmt.Fprintf(w, "_Repo: `%s` · integration branch: `%s` (via %s) · generated %s_\n\n",
		repoLabel, res.Integration, res.ElectedVia, res.GeneratedAt.Format("2006-01-02"))
	if res.ElectionNote != "" {
		fmt.Fprintf(w, "> ⚠️ %s\n\n", res.ElectionNote)
	}

	if len(res.Specs) > 0 {
		fmt.Fprintf(w, "### Spec board (intent from `.truthboard/specs`)\n\n")
		fmt.Fprintf(w, "| Status | Spec | Backlog | Title | Evidence |\n|---|---|---|---|---|\n")
		for _, st := range specStatusOrder {
			for _, s := range res.Specs {
				if s.Status != st {
					continue
				}
				title := s.Title
				if len(s.Branches) > 0 {
					title += " (`" + strings.Join(s.Branches, "`, `") + "`)"
				}
				statusCell := string(s.Status)
				if s.Status == audit.Regressed {
					statusCell = "🔴 **regressed**"
				}
				fmt.Fprintf(w, "| %s | `%s` | %s | %s | %s |\n",
					statusCell, s.ID, strings.TrimPrefix(backlogTag(s), " · "), title, s.Evidence)
			}
		}
		fmt.Fprintln(w)
	}

	if len(res.Sprints) > 0 {
		fmt.Fprintf(w, "### Sprints (derived — a sprint finishes when its stories land)\n\n")
		for _, sp := range res.Sprints {
			fmt.Fprintf(w, "- **%s** — %s", sp.Name, sprintFacts(sp))
			if len(sp.Open) > 0 {
				var open []string
				for _, o := range sp.Open {
					open = append(open, fmt.Sprintf("`%s` %s (%s)", o.ID, o.Title, o.Status))
				}
				fmt.Fprintf(w, " · open: %s", strings.Join(open, ", "))
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}

	if f := res.Flow; f.Measured() {
		fmt.Fprintf(w, "### Flow (timed from commits — nothing here was typed)\n\n")
		fmt.Fprintf(w, "_%s_\n\n", f.Headline)
		if !f.Cycle.Empty() {
			fmt.Fprintf(w, "| Measure | Median | 85th | Fastest | Slowest | Stories |\n|---|---|---|---|---|---|\n")
			row := func(name string, s audit.Stat) {
				if s.Empty() {
					return
				}
				fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %d |\n", name,
					audit.Duration(s.MedianHours), audit.Duration(s.P85Hours),
					audit.Duration(s.MinHours), audit.Duration(s.MaxHours), s.Stories)
			}
			row("Cycle (first work commit → landed)", f.Cycle)
			row("Lead (filed → landed)", f.Lead)
			fmt.Fprintln(w)
		}
		if len(f.Sprints) > 0 {
			fmt.Fprintf(w, "| Sprint | Stories landed | Points | Unestimated |\n|---|---|---|---|\n")
			for _, b := range f.Sprints {
				fmt.Fprintf(w, "| %s | %d | %d | %d |\n", b.Label, b.Stories, b.Points, b.Unestimated)
			}
			fmt.Fprintln(w)
		}
		if len(f.Unmeasurable) > 0 {
			fmt.Fprintf(w, "**Not timeable (%d)** — landed, but the history cannot say how long they took:\n\n", len(f.Unmeasurable))
			for _, u := range f.Unmeasurable {
				fmt.Fprintf(w, "- `%s` %s — %s\n", u.ID, u.Title, u.Reason)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "### Board (derived, never typed)\n\n")
	if len(res.Units) == 0 {
		fmt.Fprintf(w, "_No work-unit branches found._\n\n")
	} else {
		fmt.Fprintf(w, "| Status | Branch | Evidence |\n|---|---|---|\n")
		for _, st := range statusOrder {
			for _, u := range res.Units {
				if u.Status != st {
					continue
				}
				evidence := u.Evidence
				if len(u.Flags) > 0 {
					evidence += " — ⚠ " + strings.Join(u.Flags, "; ")
				}
				fmt.Fprintf(w, "| %s | `%s` | %s |\n", u.Status, u.Label(), evidence)
			}
		}
		fmt.Fprintln(w)
	}

	d := res.Drift
	fmt.Fprintf(w, "### Drift\n\n")
	if d.Clean() {
		fmt.Fprintf(w, "✅ Clean — the board matches reality.\n\n")
	}
	if len(d.DependencyCycles) > 0 {
		fmt.Fprintf(w, "**Dependency cycles (%d)** — intent that can never become ready:\n\n", len(d.DependencyCycles))
		for _, cy := range d.DependencyCycles {
			fmt.Fprintf(w, "- %s\n", cy)
		}
		fmt.Fprintln(w)
	}
	if len(d.UnknownRepos) > 0 {
		fmt.Fprintf(w, "**Unknown repos (%d)** — repos: intent naming repos the workspace does not declare:\n\n", len(d.UnknownRepos))
		for _, ur := range d.UnknownRepos {
			fmt.Fprintf(w, "- %s\n", ur)
		}
		fmt.Fprintln(w)
	}
	if len(d.UnwiredRepos) > 0 {
		fmt.Fprintf(w, "**Unwired spokes (%d)** — checked out and watched for proof, but agents there have no board:\n\n", len(d.UnwiredRepos))
		for _, ur := range d.UnwiredRepos {
			fmt.Fprintf(w, "- %s\n", ur)
		}
		fmt.Fprintln(w)
	}
	if len(d.ScopeCreep) > 0 {
		fmt.Fprintf(w, "**Scope creep (%d)** — linked work drifting outside declared spec paths:\n\n", len(d.ScopeCreep))
		for _, sc := range d.ScopeCreep {
			fmt.Fprintf(w, "- `%s` / `%s` — %d%% of the diff (%d/%d files) outside spec paths, mostly %s\n",
				sc.SpecID, sc.Branch, 100*sc.Outside/sc.Total, sc.Outside, sc.Total, sc.TopDirs)
		}
		fmt.Fprintln(w)
	}
	if len(d.StalePromises) > 0 {
		fmt.Fprintf(w, "**Stale promises (%d)** — work that stopped without landing:\n\n", len(d.StalePromises))
		for _, u := range d.StalePromises {
			fmt.Fprintf(w, "- `%s` — %s\n", u.Label(), u.Evidence)
		}
		fmt.Fprintln(w)
	}
	if len(d.ShadowWork) > 0 {
		fmt.Fprintf(w, "**Shadow work (%d)** — commits on `%s` outside any branch/MR flow (last %dd):\n\n",
			len(d.ShadowWork), res.Integration, res.DigestDays)
		for _, cm := range d.ShadowWork {
			fmt.Fprintf(w, "- %s%s `%s` %s: %s\n", commitTag(cm), cm.Date, cm.Hash, cm.Author, cm.Subject)
		}
		fmt.Fprintln(w)
	}
	if len(d.ContradictedHolds) > 0 {
		fmt.Fprintf(w, "**Contradicted holds (%d)** — a paused reason the evidence disagrees with:\n\n", len(d.ContradictedHolds))
		for _, h := range d.ContradictedHolds {
			fmt.Fprintf(w, "- `%s` %s — held for %q, but %s\n", h.ID, h.Title, h.Hold, h.Why)
		}
		fmt.Fprintln(w)
	}
	if len(d.UnverifiedAcceptance) > 0 {
		fmt.Fprintf(w, "**Unverified acceptance (%d)** — landed work whose criteria were never ticked:\n\n", len(d.UnverifiedAcceptance))
		for _, ua := range d.UnverifiedAcceptance {
			fmt.Fprintf(w, "- `%s` %s — %s\n", ua.ID, ua.Title, ua.Summary())
			// The open criteria are the useful part of this section: they
			// are the questions nobody has answered yet.
			for i, t := range ua.Unticked {
				if i == 5 {
					fmt.Fprintf(w, "  - … and %d more\n", len(ua.Unticked)-5)
					break
				}
				fmt.Fprintf(w, "  - [ ] %s\n", t)
			}
		}
		fmt.Fprintln(w)
	}
	if len(d.LandedNotDeleted) > 0 {
		fmt.Fprintf(w, "**Landed but branch not deleted (%d):** ", len(d.LandedNotDeleted))
		names := make([]string, len(d.LandedNotDeleted))
		for i, name := range d.LandedNotDeleted {
			names[i] = "`" + name + "`"
		}
		fmt.Fprintf(w, "%s\n\n", strings.Join(names, ", "))
	}

	if forges := forgeLabel(res); forges != "" {
		fmt.Fprintf(w, "### Claims vs proof — tracker: `%s`\n\n", forges)
		if len(res.Claims) == 0 {
			fmt.Fprintf(w, "✅ Clean — every tracker claim is backed by the repo.\n\n")
		}
		for _, kind := range claimOrder {
			shown := 0
			for _, cl := range res.Claims {
				if cl.Kind != kind {
					continue
				}
				if shown == 0 {
					fmt.Fprintf(w, "**%s:**\n\n", claimHeadlines[kind])
				}
				if shown == claimCap {
					fmt.Fprintf(w, "- … and %d more\n", countClaims(res.Claims, kind)-claimCap)
					break
				}
				fmt.Fprintf(w, "- `%s` — %s\n", cl.Subject, cl.Detail)
				shown++
			}
			if shown > 0 {
				fmt.Fprintln(w)
			}
		}
	}

	fmt.Fprintf(w, "### Landed in the last %d days\n\n", res.DigestDays)
	if len(res.Digest) == 0 {
		fmt.Fprintf(w, "_Nothing landed._\n")
	}
	for _, sh := range res.Shipped {
		tag := ""
		if sh.Epic != "" {
			tag = " · " + sh.Epic
		}
		fmt.Fprintf(w, "- ✓ **%s** (`%s`%s) — landed %s\n", sh.Title, sh.ID, tag, sh.Date)
	}
	first := true
	for _, cm := range res.Digest {
		if cm.Spec != "" {
			continue
		}
		if first && len(res.Shipped) > 0 {
			fmt.Fprintf(w, "\n**Also landed:**\n\n")
		}
		first = false
		fmt.Fprintf(w, "- %s %s\n", cm.Date, cm.Subject)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
