// Package tui renders the derived board as an interactive terminal
// dashboard (Bubbletea). It is strictly a viewer: the same Result the web
// board shows, navigated with the keyboard, with zero write paths — the
// board's statuses are derived from git and there is nothing here to set.
package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/emmanuel-D/truthboard/internal/audit"
	"github.com/emmanuel-D/truthboard/internal/spec"
)

// columnOrder is the kanban spine; empty columns outside the core four are
// hidden so a healthy board never shows a "regressed" ghost town.
var columnOrder = []audit.Status{audit.Planned, audit.InProgress, audit.InReview, audit.Done, audit.Stalled, audit.Regressed}

var coreColumns = map[audit.Status]bool{audit.Planned: true, audit.InProgress: true, audit.InReview: true, audit.Done: true}

var statusColor = map[audit.Status]lipgloss.Color{
	audit.Planned:    lipgloss.Color("245"),
	audit.InProgress: lipgloss.Color("39"),
	audit.InReview:   lipgloss.Color("135"),
	audit.Done:       lipgloss.Color("35"),
	audit.Stalled:    lipgloss.Color("178"),
	audit.Regressed:  lipgloss.Color("160"),
}

type viewMode int

const (
	viewBoard viewMode = iota
	viewDetail
	viewDrift
	viewDigest
)

// cycle is one rotating filter (epic, sprint, owner): idx -1 means off.
type cycle struct {
	values []string
	idx    int
}

func (c *cycle) current() (string, bool) {
	if c.idx < 0 || c.idx >= len(c.values) {
		return "", false
	}
	return c.values[c.idx], true
}

func (c *cycle) next() {
	if len(c.values) == 0 {
		return
	}
	c.idx++
	if c.idx >= len(c.values) {
		c.idx = -1 // wrap back to off
	}
}

type refreshMsg struct {
	res *audit.Result
	err error
}

type tickMsg time.Time

type model struct {
	repo    string
	res     *audit.Result
	err     error
	mode    viewMode
	cols    [][]audit.SpecStatus
	labels  []audit.Status
	col     int
	row     int
	width   int
	height  int
	epics   cycle
	sprints cycle
	owners  cycle

	detailBody string // spec markdown body, loaded when the detail pane opens
}

// Run opens the TUI board for repo and blocks until the user quits.
func Run(repo string) error {
	res, err := audit.Audit(repo, audit.Options{})
	if err != nil {
		return err
	}
	m := model{repo: repo, res: res, epics: cycle{idx: -1}, sprints: cycle{idx: -1}, owners: cycle{idx: -1}}
	m.rebuild()
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd { return tick() }

// tick + refresh keep the view current the same way the web board stays
// current: re-derive on a short poll, because the repo can change under us
// at any time (a merge, a push, an intent edit).
func tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) refresh() tea.Cmd {
	repo := m.repo
	return func() tea.Msg {
		res, err := audit.Audit(repo, audit.Options{})
		return refreshMsg{res: res, err: err}
	}
}

func (m *model) rebuild() {
	m.epics.values = distinct(m.res.Specs, func(s audit.SpecStatus) string { return s.Epic })
	m.sprints.values = distinct(m.res.Specs, func(s audit.SpecStatus) string { return s.Sprint })
	m.owners.values = distinct(m.res.Specs, func(s audit.SpecStatus) string { return s.Owner })

	m.cols, m.labels = nil, nil
	for _, st := range columnOrder {
		var col []audit.SpecStatus
		for _, s := range m.res.Specs {
			if s.Status == st && m.matches(s) {
				col = append(col, s)
			}
		}
		if len(col) == 0 && !coreColumns[st] {
			continue
		}
		m.cols = append(m.cols, col)
		m.labels = append(m.labels, st)
	}
	if m.col >= len(m.cols) {
		m.col = max(0, len(m.cols)-1)
	}
	m.clampRow()
}

func (m *model) matches(s audit.SpecStatus) bool {
	if v, on := m.epics.current(); on && s.Epic != v {
		return false
	}
	if v, on := m.sprints.current(); on && s.Sprint != v {
		return false
	}
	if v, on := m.owners.current(); on && s.Owner != v {
		return false
	}
	return true
}

func (m *model) clampRow() {
	if len(m.cols) == 0 {
		m.row = 0
		return
	}
	if n := len(m.cols[m.col]); m.row >= n {
		m.row = max(0, n-1)
	}
}

func (m *model) selected() *audit.SpecStatus {
	if len(m.cols) == 0 || len(m.cols[m.col]) == 0 {
		return nil
	}
	return &m.cols[m.col][m.row]
}

func distinct(specs []audit.SpecStatus, get func(audit.SpecStatus) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range specs {
		if v := get(s); v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refresh(), tick())

	case refreshMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.res = msg.res
		m.rebuild()
		return m, nil

	case tea.KeyMsg:
		switch key := msg.String(); key {
		case "ctrl+c":
			return m, tea.Quit
		case "q", "esc":
			if m.mode != viewBoard {
				m.mode = viewBoard
				return m, nil
			}
			return m, tea.Quit
		case "left", "h":
			if m.mode == viewBoard && m.col > 0 {
				m.col--
				m.clampRow()
			}
		case "right", "l":
			if m.mode == viewBoard && m.col < len(m.cols)-1 {
				m.col++
				m.clampRow()
			}
		case "up", "k":
			if m.mode == viewBoard && m.row > 0 {
				m.row--
			}
		case "down", "j":
			if m.mode == viewBoard && len(m.cols) > 0 && m.row < len(m.cols[m.col])-1 {
				m.row++
			}
		case "enter":
			if m.mode == viewBoard && m.selected() != nil {
				m.detailBody = ""
				if s, err := spec.Find(m.repo, m.selected().ID); err == nil {
					m.detailBody = s.Body
				}
				m.mode = viewDetail
			}
		case "b":
			m.mode = viewBoard
		case "d":
			m.mode = viewDrift
		case "g":
			m.mode = viewDigest
		case "e":
			m.epics.next()
			m.rebuild()
		case "s":
			m.sprints.next()
			m.rebuild()
		case "a":
			m.owners.next()
			m.rebuild()
		}
	}
	return m, nil
}

var (
	dim      = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	boldText = lipgloss.NewStyle().Bold(true)
)

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var body string
	switch m.mode {
	case viewDetail:
		body = m.viewDetail()
	case viewDrift:
		body = m.viewDrift()
	case viewDigest:
		body = m.viewDigest()
	default:
		body = m.viewBoard()
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.header(), body, m.footer())
}

func (m model) header() string {
	title := boldText.Render("truthboard") + dim.Render("  "+m.repo+" · statuses derived from git, never typed")
	if m.err != nil {
		title += "  " + lipgloss.NewStyle().Foreground(statusColor[audit.Regressed]).Render("refresh error: "+m.err.Error())
	}
	var filters []string
	if v, on := m.epics.current(); on {
		filters = append(filters, "epic="+v)
	}
	if v, on := m.sprints.current(); on {
		filters = append(filters, "sprint="+v)
	}
	if v, on := m.owners.current(); on {
		filters = append(filters, "owner="+v)
	}
	if len(filters) > 0 {
		title += "  " + lipgloss.NewStyle().Foreground(statusColor[audit.InProgress]).Render("⧩ "+strings.Join(filters, " "))
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(title)
}

func (m model) footer() string {
	help := "←→/hl columns · ↑↓/jk cards · enter detail · e epic · s sprint · a owner · b board · d drift · g digest · q quit"
	return dim.Padding(0, 1).Render(help)
}

func (m model) viewBoard() string {
	if len(m.cols) == 0 || len(m.res.Specs) == 0 {
		return dim.Padding(1, 2).Render("no specs — truthboard init, then truthboard spec new \"Title\"")
	}
	colWidth := max(24, (m.width-2)/len(m.cols)-2)
	innerH := max(6, m.height-6)

	var rendered []string
	for i, col := range m.cols {
		st := m.labels[i]
		head := lipgloss.NewStyle().Bold(true).Foreground(statusColor[st]).
			Render(fmt.Sprintf("%s %d", strings.ToUpper(string(st)), len(col)))
		cards := []string{head}
		for j, s := range col {
			cards = append(cards, m.card(s, colWidth-4, i == m.col && j == m.row))
		}
		block := lipgloss.NewStyle().
			Width(colWidth).MaxHeight(innerH).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor(i == m.col)).
			Padding(0, 1).
			Render(strings.Join(cards, "\n"))
		rendered = append(rendered, block)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func borderColor(active bool) lipgloss.Color {
	if active {
		return lipgloss.Color("39")
	}
	return lipgloss.Color("238")
}

func (m model) card(s audit.SpecStatus, width int, selected bool) string {
	title := lipgloss.NewStyle().Width(width).Render(truncate(s.Title, width))
	var tags []string
	tags = append(tags, s.ID)
	if s.Priority > 0 {
		tags = append(tags, fmt.Sprintf("p%d", s.Priority))
	}
	if s.Points > 0 {
		tags = append(tags, fmt.Sprintf("%dpt", s.Points))
	}
	if s.Type != "" && s.Type != "story" {
		tags = append(tags, s.Type)
	}
	if s.Epic != "" {
		tags = append(tags, s.Epic)
	}
	if s.Sprint != "" {
		tags = append(tags, s.Sprint)
	}
	if len(s.Waiting) > 0 {
		tags = append(tags, "⧗ waits "+strings.Join(s.Waiting, " "))
	}
	meta := dim.Render(truncate(strings.Join(tags, " · "), width))
	card := title + "\n" + meta
	style := lipgloss.NewStyle().PaddingBottom(1)
	if selected {
		style = style.Background(lipgloss.Color("236")).Bold(true)
	}
	return style.Render(card)
}

func (m model) viewDetail() string {
	s := m.selected()
	if s == nil {
		return dim.Render("nothing selected")
	}
	w := max(40, m.width-8)
	head := boldText.Render(s.Title) + "\n" +
		lipgloss.NewStyle().Foreground(statusColor[s.Status]).Render(strings.ToUpper(string(s.Status))) +
		dim.Render(" — "+s.Evidence)
	var meta []string
	for _, kv := range [][2]string{
		{"id", s.ID}, {"owner", s.Owner}, {"epic", s.Epic}, {"sprint", s.Sprint}, {"type", s.Type},
	} {
		if kv[1] != "" {
			meta = append(meta, kv[0]+": "+kv[1])
		}
	}
	if s.Priority > 0 {
		meta = append(meta, fmt.Sprintf("priority: p%d", s.Priority))
	}
	if s.Points > 0 {
		meta = append(meta, fmt.Sprintf("points: %d", s.Points))
	}
	if s.AcceptanceTotal > 0 {
		meta = append(meta, fmt.Sprintf("acceptance: %d/%d signed off", s.AcceptanceDone, s.AcceptanceTotal))
	}
	if len(s.Needs) > 0 {
		status := map[string]audit.Status{}
		for _, other := range m.res.Specs {
			status[other.ID] = other.Status
		}
		var needs []string
		for _, id := range s.Needs {
			st, ok := status[id]
			if !ok {
				st = "missing"
			}
			needs = append(needs, fmt.Sprintf("%s (%s)", id, st))
		}
		meta = append(meta, "needs: "+strings.Join(needs, ", "))
	}
	body := wrap(m.detailBody, w)
	return lipgloss.NewStyle().
		Width(min(m.width-4, w+4)).MaxHeight(max(8, m.height-5)).
		Border(lipgloss.RoundedBorder()).BorderForeground(statusColor[s.Status]).
		Padding(1, 2).
		Render(head + "\n" + dim.Render(strings.Join(meta, " · ")) + "\n\n" + body)
}

func (m model) viewDrift() string {
	d := m.res.Drift
	var b strings.Builder
	section := func(title string, n int) {
		fmt.Fprintf(&b, "%s\n", boldText.Render(fmt.Sprintf("%s (%d)", title, n)))
	}
	section("Stale promises — work that stopped without landing", len(d.StalePromises))
	for _, u := range d.StalePromises {
		fmt.Fprintf(&b, "  %s  %s\n", u.Name, dim.Render(u.Evidence))
	}
	section("Landed but branch not deleted", len(d.LandedNotDeleted))
	for _, u := range d.LandedNotDeleted {
		fmt.Fprintf(&b, "  %s\n", u.Name)
	}
	section("Shadow work — commits outside any branch/MR flow", len(d.ShadowWork))
	for i, c := range d.ShadowWork {
		if i == 12 {
			fmt.Fprintf(&b, "  %s\n", dim.Render(fmt.Sprintf("… and %d more", len(d.ShadowWork)-12)))
			break
		}
		fmt.Fprintf(&b, "  %s %s\n", dim.Render(c.Date), truncate(c.Subject, max(20, m.width-16)))
	}
	if len(d.ScopeCreep) > 0 {
		section("Scope creep", len(d.ScopeCreep))
		for _, sc := range d.ScopeCreep {
			fmt.Fprintf(&b, "  %s  %s\n", sc.SpecID,
				dim.Render(fmt.Sprintf("%s — %d/%d files outside declared paths (%s)", sc.Branch, sc.Outside, sc.Total, sc.TopDirs)))
		}
	}
	return lipgloss.NewStyle().Padding(1, 2).MaxHeight(max(8, m.height-4)).Render(b.String())
}

func (m model) viewDigest() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", boldText.Render(fmt.Sprintf("Landed on %s in the last %d days", m.res.Integration, m.res.DigestDays)))
	for _, sh := range m.res.Shipped {
		tag := ""
		if sh.Epic != "" {
			tag = " · " + sh.Epic
		}
		if sh.Type != "" && sh.Type != "story" {
			tag += " · " + sh.Type
		}
		fmt.Fprintf(&b, "  %s %s %s\n",
			lipgloss.NewStyle().Foreground(statusColor[audit.Done]).Render("✓"),
			truncate(sh.Title, max(20, m.width-30)), dim.Render(sh.Date+" · "+sh.ID+tag))
	}
	rest := 0
	for _, c := range m.res.Digest {
		if c.Spec == "" {
			rest++
		}
	}
	if rest > 0 {
		fmt.Fprintf(&b, "  %s\n", dim.Render("also landed:"))
		shown := 0
		for _, c := range m.res.Digest {
			if c.Spec != "" {
				continue
			}
			if shown == 10 {
				fmt.Fprintf(&b, "  %s\n", dim.Render(fmt.Sprintf("… and %d more", rest-10)))
				break
			}
			fmt.Fprintf(&b, "  %s %s\n", dim.Render(c.Date), truncate(c.Subject, max(20, m.width-16)))
			shown++
		}
	}
	return lipgloss.NewStyle().Padding(1, 2).MaxHeight(max(8, m.height-4)).Render(b.String())
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

// wrap does greedy word wrapping — enough for spec markdown bodies, which
// are short prose lines and checklists.
func wrap(s string, width int) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		for len([]rune(line)) > width {
			cut := width
			if i := strings.LastIndex(string([]rune(line)[:width]), " "); i > width/2 {
				cut = i
			}
			out = append(out, string([]rune(line)[:cut]))
			line = strings.TrimLeft(string([]rune(line)[cut:]), " ")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
