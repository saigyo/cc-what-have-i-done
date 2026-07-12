// Package tui provides an interactive session browser used when ccwhid is run
// with no selector flags. It presents a two-level view: a scrollable list of
// projects, then a scrollable list of that project's sessions.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

// Selection is the result of running the browser.
type Selection struct {
	Session          discovery.SessionInfo
	IncludeSubagents bool
	Redact           bool
	Open             bool
	Usage            bool
	OutDir           string
	Canceled         bool
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D97757"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#1A1A18")).Background(lipgloss.Color("#F6E7DF"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	optionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D97757"))
)

type screen int

const (
	screenProjects screen = iota
	screenSessions
	screenOptions
)

// reservedLines is how many rows the title and footer occupy, leaving the rest
// of the terminal height for the scrollable list body.
const reservedLines = 4

type model struct {
	groups []discovery.ProjectGroup

	// projects screen
	projCursor int
	projTop    int

	// sessions screen
	groupIdx   int
	showAll    bool
	sessions   []discovery.SessionInfo // filtered view of groups[groupIdx]
	sessCursor int
	sessTop    int

	// options screen
	optCursor int
	outDir    string // output directory override ("" = default); edited in place
	editing   bool   // true while the output-directory field is being typed into

	screen screen
	sel    Selection

	height int
	width  int
}

func newModel(groups []discovery.ProjectGroup) model {
	return model{
		groups: groups,
		screen: screenProjects,
		height: 20,
		width:  80,
		sel: Selection{
			IncludeSubagents: true,
			Redact:           true,
		},
	}
}

// listHeight is the number of list rows visible in the current terminal.
func (m model) listHeight() int {
	h := m.height - reservedLines
	if h < 1 {
		return 1
	}
	return h
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		m.adjustProjScroll()
		m.adjustSessScroll()
		return m, nil
	case tea.KeyMsg:
		switch m.screen {
		case screenProjects:
			return m.updateProjects(msg)
		case screenSessions:
			return m.updateSessions(msg)
		default:
			return m.updateOptions(msg)
		}
	}
	return m, nil
}

// --- projects screen ---

func (m model) updateProjects(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.sel.Canceled = true
		return m, tea.Quit
	case "up", "k":
		if m.projCursor > 0 {
			m.projCursor--
			m.adjustProjScroll()
		}
	case "down", "j":
		if m.projCursor < len(m.groups)-1 {
			m.projCursor++
			m.adjustProjScroll()
		}
	case "enter", "right", "l":
		if len(m.groups) > 0 {
			m.enterProject(m.projCursor)
		}
	}
	return m, nil
}

func (m *model) adjustProjScroll() {
	m.projTop = clampScroll(m.projCursor, m.projTop, m.listHeight(), len(m.groups))
}

func (m *model) enterProject(idx int) {
	m.groupIdx = idx
	m.showAll = false
	m.sessCursor = 0
	m.sessTop = 0
	m.refreshSessions()
	m.screen = screenSessions
}

func (m *model) refreshSessions() {
	g := m.groups[m.groupIdx]
	if m.showAll {
		m.sessions = g.Sessions
	} else {
		m.sessions = g.RootSessions()
	}
	if m.sessCursor >= len(m.sessions) {
		m.sessCursor = max(0, len(m.sessions)-1)
	}
	m.adjustSessScroll()
}

// --- sessions screen ---

func (m model) updateSessions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q":
		m.sel.Canceled = true
		return m, tea.Quit
	case "esc", "left", "h":
		m.screen = screenProjects
	case "up", "k":
		if m.sessCursor > 0 {
			m.sessCursor--
			m.adjustSessScroll()
		}
	case "down", "j":
		if m.sessCursor < len(m.sessions)-1 {
			m.sessCursor++
			m.adjustSessScroll()
		}
	case "a":
		m.showAll = !m.showAll
		m.sessCursor = 0
		m.sessTop = 0
		m.refreshSessions()
	case "enter", "right", "l":
		if len(m.sessions) > 0 {
			m.sel.Session = m.sessions[m.sessCursor]
			m.optCursor = 0
			m.screen = screenOptions
		}
	}
	return m, nil
}

func (m *model) adjustSessScroll() {
	m.sessTop = clampScroll(m.sessCursor, m.sessTop, m.listHeight(), len(m.sessions))
}

// --- options screen ---

// Option-screen row indices.
const (
	optSubagents = iota
	optRedact
	optOpen
	optUsage
	optOutDir
	optGenerate
	optionCount
)

func (m model) updateOptions(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While editing the output-directory field, keys feed the text buffer;
	// only Ctrl+C still quits so typed characters (q, h, …) are taken literally.
	if m.editing {
		switch key.Type {
		case tea.KeyCtrlC:
			m.sel.Canceled = true
			return m, tea.Quit
		case tea.KeyEnter, tea.KeyEsc:
			m.editing = false
		case tea.KeyBackspace, tea.KeyDelete:
			if r := []rune(m.outDir); len(r) > 0 {
				m.outDir = string(r[:len(r)-1])
			}
		case tea.KeyCtrlU:
			m.outDir = ""
		case tea.KeySpace:
			m.outDir += " "
		case tea.KeyRunes:
			m.outDir += string(key.Runes)
		}
		return m, nil
	}

	switch key.String() {
	case "ctrl+c", "q":
		m.sel.Canceled = true
		return m, tea.Quit
	case "esc", "left", "h":
		m.screen = screenSessions
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
		case optSubagents:
			m.sel.IncludeSubagents = !m.sel.IncludeSubagents
		case optRedact:
			m.sel.Redact = !m.sel.Redact
		case optOpen:
			m.sel.Open = !m.sel.Open
		case optUsage:
			m.sel.Usage = !m.sel.Usage
		case optOutDir:
			m.editing = true
		case optGenerate:
			m.sel.OutDir = strings.TrimSpace(m.outDir)
			return m, tea.Quit
		}
	}
	return m, nil
}

// clampScroll returns a scroll offset that keeps cursor within the visible
// window [top, top+visible) over a list of length n.
func clampScroll(cursor, top, visible, n int) int {
	if visible < 1 {
		visible = 1
	}
	if cursor < top {
		top = cursor
	}
	if cursor >= top+visible {
		top = cursor - visible + 1
	}
	if top < 0 {
		top = 0
	}
	// Don't scroll past the end when the list shrank.
	if max := n - visible; top > max && max >= 0 {
		top = max
	}
	return top
}

func (m model) View() string {
	switch m.screen {
	case screenProjects:
		return m.viewProjects()
	case screenSessions:
		return m.viewSessions()
	default:
		return m.viewOptions()
	}
}

func (m model) viewProjects() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select a project") +
		mutedStyle.Render("   ↑/↓ move · enter open · q quit") + "\n\n")
	visible := m.listHeight()
	end := min(m.projTop+visible, len(m.groups))
	for i := m.projTop; i < end; i++ {
		g := m.groups[i]
		roots, agents := g.Counts()
		count := fmt.Sprintf("%d session%s", roots, plural(roots))
		if agents > 0 {
			count += fmt.Sprintf(", %d agent%s", agents, plural(agents))
		}
		label := headerStyle.Render(g.ProjectPath) + "  " + mutedStyle.Render(count)
		if i == m.projCursor {
			label = selectedStyle.Render("▸ " + g.ProjectPath + "  " + count)
		} else {
			label = "  " + label
		}
		b.WriteString(label + "\n")
	}
	b.WriteString(scrollHint(m.projTop, end, len(m.groups)))
	return b.String()
}

func (m model) viewSessions() string {
	var b strings.Builder
	g := m.groups[m.groupIdx]
	b.WriteString(titleStyle.Render("Sessions in ") + headerStyle.Render(g.ProjectPath) + "\n")
	toggle := "a show all"
	if m.showAll {
		toggle = "a root only"
	}
	b.WriteString(mutedStyle.Render("↑/↓ move · enter select · ← back · "+toggle+" · q quit") + "\n\n")

	if len(m.sessions) == 0 {
		b.WriteString(mutedStyle.Render("  no interactive sessions here — press a to show agent sessions") + "\n")
		return b.String()
	}
	visible := m.listHeight()
	end := min(m.sessTop+visible, len(m.sessions))
	for i := m.sessTop; i < end; i++ {
		s := m.sessions[i]
		id := s.ID[:min(8, len(s.ID))]
		tag := ""
		if s.IsAgent {
			tag = mutedStyle.Render(" ⟲")
		}
		line := fmt.Sprintf("  %s  %s%s", s.DisplayLabel(), mutedStyle.Render(id), tag)
		if i == m.sessCursor {
			line = selectedStyle.Render("▸ "+s.DisplayLabel()+"  "+id) + tag
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(scrollHint(m.sessTop, end, len(m.sessions)))
	return b.String()
}

func (m model) viewOptions() string {
	var b strings.Builder
	b.WriteString("Options for: " + m.sel.Session.DisplayLabel() + "\n\n")
	toggles := []struct {
		label string
		on    bool
	}{
		{"Include subagent work", m.sel.IncludeSubagents},
		{"Redact secrets", m.sel.Redact},
		{"Open in browser when done", m.sel.Open},
		{"Include usage & cost", m.sel.Usage},
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

	// Output-directory field.
	outRow := "Output dir: " + m.outDirDisplay()
	if m.optCursor == optOutDir {
		outRow = optionStyle.Render("▸ " + outRow)
	} else {
		outRow = "  " + outRow
	}
	b.WriteString(outRow + "\n")

	gen := "Generate report"
	if m.optCursor == optGenerate {
		gen = optionStyle.Render("▸ " + gen)
	} else {
		gen = "  " + gen
	}
	b.WriteString("\n" + gen + "\n\n")

	help := "↑/↓ move · space/enter select · esc back · q quit"
	if m.editing {
		help = "type a path · enter/esc done · ctrl+u clear"
	}
	b.WriteString(mutedStyle.Render(help) + "\n")
	return b.String()
}

// outDirDisplay renders the current output-directory value: the typed path (with
// a cursor while editing), or a muted hint showing the default destination.
func (m model) outDirDisplay() string {
	if m.editing {
		return m.outDir + "▏"
	}
	if m.outDir != "" {
		return m.outDir
	}
	short := m.sel.Session.ID
	if len(short) > 8 {
		short = short[:8]
	}
	return mutedStyle.Render("ccwhid-report/" + short + " (default)")
}

// scrollHint renders a one-line position indicator when the list is scrolled or
// overflows the viewport.
func scrollHint(top, end, n int) string {
	if top == 0 && end >= n {
		return ""
	}
	return mutedStyle.Render(fmt.Sprintf("\n  %d–%d of %d", top+1, end, n))
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Run launches the browser and returns the user's selection. If focusIdx is a
// valid index into groups, the browser opens directly on that project's session
// list; pass -1 to start on the project list.
func Run(groups []discovery.ProjectGroup, focusIdx int) (Selection, error) {
	if len(groups) == 0 {
		return Selection{Canceled: true}, fmt.Errorf("no sessions found under ~/.claude/projects")
	}
	m := newModel(groups)
	if focusIdx >= 0 && focusIdx < len(groups) {
		m.projCursor = focusIdx
		m.enterProject(focusIdx)
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	res, err := p.Run()
	if err != nil {
		return Selection{}, err
	}
	fm := res.(model)
	return fm.sel, nil
}
