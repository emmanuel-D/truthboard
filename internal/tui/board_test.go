package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/emmanuel-D/truthboard/internal/audit"
)

func fakeModel() model {
	res := &audit.Result{
		Integration: "main",
		DigestDays:  14,
		Specs: []audit.SpecStatus{
			{ID: "tb-0001", Title: "Landed story", Status: audit.Done, Epic: "core", Sprint: "s12", Points: 5, Evidence: "work landed on main"},
			{ID: "tb-0002", Title: "Open story", Status: audit.Planned, Epic: "core", Owner: "ada", Evidence: "no matching branch or commit yet"},
			{ID: "tb-0003", Title: "A bug elsewhere", Status: audit.Planned, Epic: "ui", Type: "bug", Evidence: "no matching branch or commit yet"},
		},
		Shipped: []audit.ShippedSpec{{ID: "tb-0001", Title: "Landed story", Epic: "core", Date: "2026-07-16"}},
	}
	m := model{repo: "/tmp/x", res: res, epics: cycle{idx: -1}, sprints: cycle{idx: -1}, owners: cycle{idx: -1}}
	m.width, m.height = 120, 40
	m.rebuild()
	return m
}

func key(m model, k string) model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
	return next.(model)
}

func TestBoardRendersColumnsAndCards(t *testing.T) {
	v := fakeModel().View()
	for _, want := range []string{"PLANNED 2", "DONE 1", "Landed story", "A bug elsewhere", "5pt", "bug", "derived from git"} {
		if !strings.Contains(v, want) {
			t.Errorf("board view missing %q", want)
		}
	}
}

func TestNavigationAndDetail(t *testing.T) {
	m := fakeModel()
	// Column 0 is planned (backlog order: tb-0002, tb-0003 by input order).
	m = key(m, "j")
	sel := m.selected()
	if sel == nil || sel.ID != m.cols[0][1].ID {
		t.Fatalf("after j, selected = %+v, want second card of first column", sel)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.mode != viewDetail {
		t.Fatalf("enter should open the detail pane, mode = %v", m.mode)
	}
	v := m.View()
	if !strings.Contains(v, sel.Title) || !strings.Contains(v, "PLANNED") {
		t.Errorf("detail view missing title/status:\n%s", v)
	}
	m = key(m, "q") // first q closes the pane, not the program
	if m.mode != viewBoard {
		t.Errorf("q in detail should return to board, mode = %v", m.mode)
	}
}

func TestFilterCyclesNarrowTheBoard(t *testing.T) {
	m := fakeModel()
	m = key(m, "e") // first epic: core
	if v, on := m.epics.current(); !on || v != "core" {
		t.Fatalf("epic filter = %q on=%v, want core", v, on)
	}
	for _, col := range m.cols {
		for _, s := range col {
			if s.Epic != "core" {
				t.Errorf("card %s escaped the epic filter", s.ID)
			}
		}
	}
	m = key(m, "e") // ui
	m = key(m, "e") // wraps to off
	if _, on := m.epics.current(); on {
		t.Error("third press should cycle the filter off")
	}
	m = key(m, "a") // owner: ada
	total := 0
	for _, col := range m.cols {
		total += len(col)
	}
	if total != 1 {
		t.Errorf("owner filter left %d cards, want 1", total)
	}
}

func TestDriftAndDigestViews(t *testing.T) {
	m := fakeModel()
	m = key(m, "g")
	if v := m.View(); !strings.Contains(v, "Landed on main") || !strings.Contains(v, "Landed story") {
		t.Errorf("digest view incomplete:\n%s", v)
	}
	m = key(m, "d")
	if v := m.View(); !strings.Contains(v, "Stale promises") || !strings.Contains(v, "Shadow work") {
		t.Errorf("drift view incomplete:\n%s", v)
	}
}

// workspaceModel is fakeModel plus a two-spoke workspace: one story landed
// in the api spoke, one declared for web via repos:, one hub-only.
func workspaceModel() model {
	res := &audit.Result{
		Integration: "main",
		DigestDays:  14,
		Workspace: []audit.RepoHealth{
			{Name: "api", Path: "/tmp/api", Integration: "main"},
			{Name: "web", Path: "/tmp/web", Integration: "main"},
		},
		Specs: []audit.SpecStatus{
			{ID: "tb-0001", Title: "Api story", Status: audit.Done, Landed: "abc1234", LandedRepo: "api", Evidence: "work landed on api:main"},
			{ID: "tb-0002", Title: "Web story", Status: audit.Planned, Repos: []string{"web"},
				PerRepo: []audit.RepoLanding{{Repo: "web", State: "missing"}}, Evidence: "web — no branch yet"},
			{ID: "tb-0003", Title: "Hub story", Status: audit.InProgress, Branches: []string{"feature/tb-0003-x"}, Evidence: "active"},
		},
	}
	m := model{repo: "/tmp/x", res: res, epics: cycle{idx: -1}, sprints: cycle{idx: -1}, owners: cycle{idx: -1}, repos: cycle{idx: -1}}
	m.width, m.height = 120, 40
	m.rebuild()
	return m
}

func TestRepoCycleNarrowsWorkspaceBoard(t *testing.T) {
	m := workspaceModel()
	if !strings.Contains(m.footer(), "r repo") {
		t.Error("workspace footer should offer the r key")
	}

	m = key(m, "r") // hub
	if v, on := m.repos.current(); !on || v != "hub" {
		t.Fatalf("repo filter = %q on=%v, want hub first", v, on)
	}
	if total := cardCount(m); total != 1 {
		t.Errorf("hub filter left %d cards, want 1 (the hub-branch story)", total)
	}
	if !strings.Contains(m.header(), "repo=hub") {
		t.Errorf("header should show the repo filter, got %q", m.header())
	}

	m = key(m, "r") // api: the landed story
	if ids := visibleIDs(m); len(ids) != 1 || ids[0] != "tb-0001" {
		t.Errorf("api filter should show exactly tb-0001, got %v", ids)
	}

	m = key(m, "r") // web: the repos: story
	if ids := visibleIDs(m); len(ids) != 1 || ids[0] != "tb-0002" {
		t.Errorf("web filter should show exactly the repos: story, got %v", ids)
	}

	m = key(m, "r") // wraps to off
	if _, on := m.repos.current(); on {
		t.Error("fourth press should cycle the repo filter off")
	}
}

func TestSingleRepoBoardHasNoRepoDimension(t *testing.T) {
	m := fakeModel()
	if strings.Contains(m.footer(), "r repo") {
		t.Error("single-repo footer must not mention the repo key")
	}
	before := cardCount(m)
	m = key(m, "r")
	if _, on := m.repos.current(); on {
		t.Error("r must be inert without a workspace")
	}
	if cardCount(m) != before {
		t.Error("r changed the board on a single-repo project")
	}
}

func cardCount(m model) int {
	return len(visibleIDs(m))
}

func visibleIDs(m model) []string {
	var ids []string
	for _, col := range m.cols {
		for _, s := range col {
			ids = append(ids, s.ID)
		}
	}
	return ids
}
