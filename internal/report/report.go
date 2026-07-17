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
			fmt.Fprintf(w, "  %s  %d/%d done%s\n", c(ansiCyan, sp.Name), sp.Done, sp.Total, c(ansiDim, sprintWindow(sp)))
			for _, o := range sp.Open {
				fmt.Fprintf(w, "    %s %s %s\n",
					c(ansi[o.Status], fmt.Sprintf("%-12s", strings.ToUpper(string(o.Status)))), o.ID, o.Title)
			}
		}
	}

	fmt.Fprintf(w, "\n%s\n", c(ansiBold, "DERIVED BOARD (no human ever set these statuses)"))
	width := 10
	for _, u := range res.Units {
		if len(u.Name) > width {
			width = len(u.Name)
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
				width, u.Name, c(ansiDim, u.Evidence))
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
			fmt.Fprintf(w, "    - %s: %s\n", u.Name, u.Evidence)
		}
	}
	if len(d.LandedNotDeleted) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiDim, fmt.Sprintf("  Landed but branch not deleted (%d):", len(d.LandedNotDeleted))))
		for _, u := range d.LandedNotDeleted {
			fmt.Fprintf(w, "    - %s\n", u.Name)
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
			fmt.Fprintf(w, "    - %s %s %s: %s\n", cm.Date, cm.Hash, cm.Author, truncate(cm.Subject, 70))
		}
	}
	if len(d.ScopeCreep) > 0 {
		fmt.Fprintf(w, "%s\n", c(ansiYellow, fmt.Sprintf("  Scope creep (%d): linked work drifting outside declared spec paths", len(d.ScopeCreep))))
		for _, sc := range d.ScopeCreep {
			fmt.Fprintf(w, "    - %s / %s: %d%% of the diff (%d/%d files) outside spec paths — mostly %s\n",
				sc.SpecID, sc.Branch, 100*sc.Outside/sc.Total, sc.Outside, sc.Total, sc.TopDirs)
		}
	}
	if len(d.StalePromises) == 0 && len(d.ShadowWork) == 0 && len(d.ScopeCreep) == 0 {
		fmt.Fprintf(w, "%s\n", c(ansiGreen, "  clean — board matches reality"))
	}

	if res.Forge != "" {
		fmt.Fprintf(w, "\n%s\n", c(ansiBold, fmt.Sprintf("CLAIMS vs PROOF — tracker: %s", res.Forge)))
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
	for _, sh := range res.Shipped {
		tag := ""
		if sh.Epic != "" {
			tag = " · " + sh.Epic
		}
		fmt.Fprintf(w, "  %s %s (%s%s)\n", c(ansiGreen, "✓ "+sh.Title), c(ansiDim, "landed "+sh.Date), sh.ID, tag)
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
		fmt.Fprintf(w, "  %s %s\n", cm.Date, truncate(cm.Subject, 80))
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
			fmt.Fprintf(w, "- **%s** — %d/%d done", sp.Name, sp.Done, sp.Total)
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
				fmt.Fprintf(w, "| %s | `%s` | %s |\n", u.Status, u.Name, evidence)
			}
		}
		fmt.Fprintln(w)
	}

	d := res.Drift
	fmt.Fprintf(w, "### Drift\n\n")
	if len(d.StalePromises) == 0 && len(d.ShadowWork) == 0 && len(d.ScopeCreep) == 0 {
		fmt.Fprintf(w, "✅ Clean — the board matches reality.\n\n")
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
			fmt.Fprintf(w, "- `%s` — %s\n", u.Name, u.Evidence)
		}
		fmt.Fprintln(w)
	}
	if len(d.ShadowWork) > 0 {
		fmt.Fprintf(w, "**Shadow work (%d)** — commits on `%s` outside any branch/MR flow (last %dd):\n\n",
			len(d.ShadowWork), res.Integration, res.DigestDays)
		for _, cm := range d.ShadowWork {
			fmt.Fprintf(w, "- %s `%s` %s: %s\n", cm.Date, cm.Hash, cm.Author, cm.Subject)
		}
		fmt.Fprintln(w)
	}
	if len(d.LandedNotDeleted) > 0 {
		fmt.Fprintf(w, "**Landed but branch not deleted (%d):** ", len(d.LandedNotDeleted))
		names := make([]string, len(d.LandedNotDeleted))
		for i, u := range d.LandedNotDeleted {
			names[i] = "`" + u.Name + "`"
		}
		fmt.Fprintf(w, "%s\n\n", strings.Join(names, ", "))
	}

	if res.Forge != "" {
		fmt.Fprintf(w, "### Claims vs proof — tracker: `%s`\n\n", res.Forge)
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
