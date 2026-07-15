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

var statusOrder = []audit.Status{audit.InProgress, audit.Stalled, audit.Done}

var ansi = map[audit.Status]string{
	audit.InProgress: "\033[36m",
	audit.Stalled:    "\033[33m",
	audit.Done:       "\033[32m",
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
	if len(d.StalePromises) == 0 && len(d.ShadowWork) == 0 {
		fmt.Fprintf(w, "%s\n", c(ansiGreen, "  clean — board matches reality"))
	}

	fmt.Fprintf(w, "\n%s\n", c(ansiBold, fmt.Sprintf("DIGEST — what landed on %s in the last %d days", res.Integration, res.DigestDays)))
	for i, cm := range res.Digest {
		if i == 20 {
			fmt.Fprintf(w, "  … and %d more\n", len(res.Digest)-20)
			break
		}
		fmt.Fprintf(w, "  %s %s\n", cm.Date, truncate(cm.Subject, 80))
	}
	if len(res.Digest) == 0 {
		fmt.Fprintln(w, "  nothing landed")
	}
	fmt.Fprintln(w)
	return nil
}

func Markdown(w io.Writer, res *audit.Result) error {
	fmt.Fprintf(w, "## Truthboard drift report\n\n")
	fmt.Fprintf(w, "_Repo: `%s` · integration branch: `%s` (via %s) · generated %s_\n\n",
		res.Repo, res.Integration, res.ElectedVia, res.GeneratedAt.Format("2006-01-02"))
	if res.ElectionNote != "" {
		fmt.Fprintf(w, "> ⚠️ %s\n\n", res.ElectionNote)
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
	if len(d.StalePromises) == 0 && len(d.ShadowWork) == 0 {
		fmt.Fprintf(w, "✅ Clean — the board matches reality.\n\n")
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

	fmt.Fprintf(w, "### Landed in the last %d days\n\n", res.DigestDays)
	if len(res.Digest) == 0 {
		fmt.Fprintf(w, "_Nothing landed._\n")
	}
	for _, cm := range res.Digest {
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
