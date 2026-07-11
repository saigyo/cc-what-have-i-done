// Package tui provides an interactive session browser used when ccwhid is run
// with no selector flags.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

// Row is one line in the browser: either a project header or a session.
type Row struct {
	IsHeader bool
	Label    string
	Session  discovery.SessionInfo
}

// Selection is the result of running the browser.
type Selection struct {
	Session          discovery.SessionInfo
	IncludeSubagents bool
	Redact           bool
	Open             bool
	OutDir           string
	Canceled         bool
}

func flattenRows(groups []discovery.ProjectGroup) []Row {
	var rows []Row
	for _, g := range groups {
		rows = append(rows, Row{IsHeader: true, Label: g.ProjectPath})
		for _, s := range g.Sessions {
			rows = append(rows, Row{Session: s})
		}
	}
	return rows
}

func firstSelectable(rows []Row) int {
	for i, r := range rows {
		if !r.IsHeader {
			return i
		}
	}
	return 0
}

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D97757")).MarginTop(1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A18")).Background(lipgloss.Color("#F6E7DF"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	optionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97757"))
)

type screen int

const (
	screenList screen = iota
	screenOptions
)

type model struct {
	rows      []Row
	cursor    int
	screen    screen
	sel       Selection
	optCursor int
}

func newModel(groups []discovery.ProjectGroup) model {
	rows := flattenRows(groups)
	return model{
		rows:   rows,
		cursor: firstSelectable(rows),
		sel: Selection{
			IncludeSubagents: true,
			Redact:           true,
		},
	}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.screen {
	case screenList:
		return m.updateList(key)
	default:
		return m.updateOptions(key)
	}
}

func (m model) updateList(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.sel.Canceled = true
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter":
		if !m.rows[m.cursor].IsHeader {
			m.sel.Session = m.rows[m.cursor].Session
			m.screen = screenOptions
		}
	}
	return m, nil
}

func (m *model) moveCursor(delta int) {
	i := m.cursor
	for {
		i += delta
		if i < 0 || i >= len(m.rows) {
			return // out of bounds; keep current
		}
		if !m.rows[i].IsHeader {
			m.cursor = i
			return
		}
	}
}

// optionCount is the number of toggle rows plus the Generate action.
const optionCount = 4 // subagents, redact, open, generate

func (m model) updateOptions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q":
		m.sel.Canceled = true
		return m, tea.Quit
	case "esc":
		m.screen = screenList
	case "up", "k":
		if m.optCursor > 0 {
			m.optCursor--
		}
	case "down", "j":
		if m.optCursor < optionCount-1 {
			m.optCursor++
		}
	case " ", "enter":
		switch m.optCursor {
		case 0:
			m.sel.IncludeSubagents = !m.sel.IncludeSubagents
		case 1:
			m.sel.Redact = !m.sel.Redact
		case 2:
			m.sel.Open = !m.sel.Open
		case 3:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	if m.screen == screenOptions {
		return m.viewOptions()
	}
	return m.viewList()
}

func (m model) viewList() string {
	var b strings.Builder
	b.WriteString("Select a session  ↑/↓ move · enter select · q quit\n")
	for i, r := range m.rows {
		if r.IsHeader {
			b.WriteString(headerStyle.Render(r.Label) + "\n")
			continue
		}
		line := fmt.Sprintf("  %s  %s", r.Session.DisplayLabel(), mutedStyle.Render(r.Session.ID[:min(8, len(r.Session.ID))]))
		if i == m.cursor {
			line = selectedStyle.Render("▸ " + strings.TrimLeft(line, " "))
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (m model) viewOptions() string {
	var b strings.Builder
	b.WriteString("Options for: " + m.sel.Session.DisplayLabel() + "\n\n")
	toggles := []struct {
		label string
		on    bool
	}{
		{"Include subagents", m.sel.IncludeSubagents},
		{"Redact secrets", m.sel.Redact},
		{"Open in browser when done", m.sel.Open},
	}
	for i, t := range toggles {
		check := "[ ]"
		if t.on {
			check = "[x]"
		}
		line := fmt.Sprintf("%s %s", check, t.label)
		if m.optCursor == i {
			line = optionStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	gen := "Generate report"
	if m.optCursor == 3 {
		gen = optionStyle.Render("▸ " + gen)
	} else {
		gen = "  " + gen
	}
	b.WriteString("\n" + gen + "\n\n")
	b.WriteString(mutedStyle.Render("space toggle · enter confirm · esc back · q quit") + "\n")
	return b.String()
}

// Run launches the browser and returns the user's selection.
func Run(groups []discovery.ProjectGroup) (Selection, error) {
	if len(groups) == 0 {
		return Selection{Canceled: true}, fmt.Errorf("no sessions found under ~/.claude/projects")
	}
	m := newModel(groups)
	p := tea.NewProgram(m)
	res, err := p.Run()
	if err != nil {
		return Selection{}, err
	}
	fm := res.(model)
	return fm.sel, nil
}
