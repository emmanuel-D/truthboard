package importer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/emmanuel-D/truthboard/internal/gitrepo"
)

// FromGitHub reads the repository's open and closed issues through gh.
// Pull requests are excluded by gh's own issue list, which is what we want:
// a PR is work in flight, not a promise somebody made.
func FromGitHub(repo string) ([]Item, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("importing from GitHub needs the `gh` CLI on PATH")
	}
	cmd := exec.Command("gh", "issue", "list", "--state", "all", "--limit", "1000",
		"--json", "number,title,body,state,labels,assignees,url")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		msg := err.Error()
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("gh issue list: %s", gitrepo.Redact(msg))
	}

	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		URL    string `json:"url"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		Assignees []struct {
			Login string `json:"login"`
		} `json:"assignees"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("reading issues from gh: %w", err)
	}

	items := make([]Item, 0, len(raw))
	for _, r := range raw {
		it := Item{
			Key: fmt.Sprintf("github#%d", r.Number), Title: r.Title, Body: r.Body,
			State: strings.ToLower(r.State), URL: r.URL,
		}
		for _, l := range r.Labels {
			it.Labels = append(it.Labels, l.Name)
			if p := priorityOf(l.Name); p != 0 && it.Priority == 0 {
				it.Priority = p
			}
		}
		if len(r.Assignees) > 0 {
			it.Owner = r.Assignees[0].Login
		}
		items = append(items, it)
	}
	return items, nil
}

// FromFile reads an export. JSON (an array of objects) and CSV (a header
// row) are both accepted, because Jira and Linear both export CSV and
// everything else that matters can produce JSON.
//
// The column names are the documented contract, and they are generous on
// purpose: every tracker spells the same six ideas differently, and making
// somebody rename columns before they can adopt a tool is a reason not to
// adopt the tool.
func FromFile(path string) ([]Item, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if trimmed := strings.TrimLeft(string(raw), " \t\r\n"); strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
		return fromJSON(raw)
	}
	return fromCSV(raw)
}

func fromJSON(raw []byte) ([]Item, error) {
	// An array, or an object with an "issues"/"items" array — both shapes
	// come out of real exports.
	var direct []map[string]any
	if err := json.Unmarshal(raw, &direct); err == nil {
		return itemsFromMaps(direct)
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("this file is not a JSON array of items, nor an object containing one: %w", err)
	}
	for _, key := range []string{"issues", "items", "records", "results"} {
		if inner, ok := wrapped[key]; ok {
			var rows []map[string]any
			if err := json.Unmarshal(inner, &rows); err == nil {
				return itemsFromMaps(rows)
			}
		}
	}
	return nil, fmt.Errorf("no array of items found — expected a JSON array, or an object with an \"issues\" or \"items\" array")
}

func fromCSV(raw []byte) ([]Item, error) {
	r := csv.NewReader(strings.NewReader(string(raw)))
	r.FieldsPerRecord = -1 // exports are ragged; a short row is not a parse error
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("this CSV has a header and no rows")
	}
	header := rows[0]
	var out []map[string]any
	for _, row := range rows[1:] {
		m := map[string]any{}
		for i, cell := range row {
			if i < len(header) {
				m[header[i]] = cell
			}
		}
		out = append(out, m)
	}
	return itemsFromMaps(out)
}

// aliases are the column names each field answers to. Order matters: the
// first present wins.
var aliases = map[string][]string{
	"key":      {"key", "id", "issue key", "identifier", "number", "issue id"},
	"title":    {"title", "summary", "name", "subject"},
	"body":     {"body", "description", "details", "notes"},
	"owner":    {"owner", "assignee", "assigned to", "assignee name"},
	"labels":   {"labels", "label", "epic", "components", "tags", "team"},
	"priority": {"priority", "importance", "severity"},
	"state":    {"state", "status", "resolution"},
	"url":      {"url", "link", "issue url", "permalink"},
}

func itemsFromMaps(rows []map[string]any) ([]Item, error) {
	var items []Item
	for i, row := range rows {
		lower := make(map[string]any, len(row))
		for k, v := range row {
			lower[strings.ToLower(strings.TrimSpace(k))] = v
		}
		it := Item{
			Key:   text(lower, "key"),
			Title: text(lower, "title"),
			Body:  text(lower, "body"),
			Owner: text(lower, "owner"),
			URL:   text(lower, "url"),
			State: normaliseState(text(lower, "state")),
		}
		if it.Key == "" {
			// Provenance is what makes a re-import safe, so an item without
			// one gets a stable synthetic key from its position and title
			// rather than being silently unrepeatable.
			it.Key = fmt.Sprintf("import:%d-%s", i+1, slug(it.Title))
		}
		if labels := text(lower, "labels"); labels != "" {
			for _, l := range strings.Split(labels, ",") {
				if l = strings.TrimSpace(l); l != "" {
					it.Labels = append(it.Labels, l)
				}
			}
		}
		it.Priority = priorityOf(text(lower, "priority"))
		items = append(items, it)
	}
	return items, nil
}

func text(row map[string]any, field string) string {
	for _, name := range aliases[field] {
		v, ok := row[name]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if s := strings.TrimSpace(t); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(t)
		}
	}
	return ""
}

// normaliseState reduces every tracker's vocabulary to the one distinction
// import cares about: is this item finished in the source, and therefore
// not worth carrying by default.
func normaliseState(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "closed", "done", "resolved", "completed", "cancelled", "canceled", "wontfix", "won't do", "duplicate":
		return "closed"
	case "":
		return ""
	default:
		return "open"
	}
}

// priorityOf maps the ways trackers spell urgency onto 1/2/3. Anything it
// does not recognise is left unset: an invented priority would reorder
// somebody's backlog on import, which is exactly the kind of quiet damage
// this tool exists to prevent.
func priorityOf(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "p1", "highest", "urgent", "critical", "blocker", "high":
		return 1
	case "2", "p2", "medium", "normal", "major":
		return 2
	case "3", "p3", "low", "minor", "lowest", "trivial":
		return 3
	default:
		return 0
	}
}
