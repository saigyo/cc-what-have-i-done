package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

func testGroups() []discovery.ProjectGroup {
	return []discovery.ProjectGroup{
		{
			ProjectPath: "/tmp/projA",
			Sessions: []discovery.SessionInfo{
				{ID: "root1aaa-0000", Title: "Root one"},
				{ID: "agent1aa-0000", Title: "Agent one", IsAgent: true},
				{ID: "root2aaa-0000", Title: "Root two"},
			},
		},
		{
			ProjectPath: "/tmp/projB",
			Sessions:    []discovery.SessionInfo{{ID: "root3bbb-0000", Title: "Root three"}},
		},
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func send(m model, msgs ...tea.Msg) model {
	var tm tea.Model = m
	for _, msg := range msgs {
		tm, _ = tm.Update(msg)
	}
	return tm.(model)
}

func TestSessionListHidesAgentsByDefault(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter")) // open projA
	if m.screen != screenSessions {
		t.Fatalf("screen = %v, want sessions", m.screen)
	}
	if len(m.sessions) != 2 {
		t.Fatalf("root sessions = %d, want 2 (agents hidden)", len(m.sessions))
	}
	for _, s := range m.sessions {
		if s.IsAgent {
			t.Errorf("agent session %q leaked into default list", s.ID)
		}
	}
}

func TestToggleShowAllRevealsAgents(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("a"))
	if !m.showAll {
		t.Fatal("showAll not toggled")
	}
	if len(m.sessions) != 3 {
		t.Fatalf("all sessions = %d, want 3", len(m.sessions))
	}
	m = send(m, key("a"))
	if m.showAll || len(m.sessions) != 2 {
		t.Fatalf("toggle back failed: showAll=%v n=%d", m.showAll, len(m.sessions))
	}
}

func TestLeftReturnsToProjectList(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"))
	if m.screen != screenSessions {
		t.Fatalf("screen = %v, want sessions", m.screen)
	}
	m = send(m, key("left"))
	if m.screen != screenProjects {
		t.Fatalf("screen = %v, want projects after left", m.screen)
	}
}

func TestSelectSessionGoesToOptions(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("enter")) // open projA, select first session
	if m.screen != screenOptions {
		t.Fatalf("screen = %v, want options", m.screen)
	}
	if m.sel.Session.ID != "root1aaa-0000" {
		t.Errorf("selected %q, want root1aaa-0000", m.sel.Session.ID)
	}
}

func TestScrollKeepsCursorVisible(t *testing.T) {
	// 30 sessions, viewport of 10 rows.
	var ss []discovery.SessionInfo
	for i := 0; i < 30; i++ {
		ss = append(ss, discovery.SessionInfo{ID: string(rune('a'+i%26)) + "0000000", Title: "s"})
	}
	m := newModel([]discovery.ProjectGroup{{ProjectPath: "/p", Sessions: ss}})
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: reservedLines + 10}, key("enter"))
	for i := 0; i < 25; i++ {
		m = send(m, key("down"))
	}
	if m.sessCursor != 25 {
		t.Fatalf("cursor = %d, want 25", m.sessCursor)
	}
	// cursor must be within the visible window [top, top+10)
	if m.sessCursor < m.sessTop || m.sessCursor >= m.sessTop+m.listHeight() {
		t.Errorf("cursor %d outside window [%d,%d)", m.sessCursor, m.sessTop, m.sessTop+m.listHeight())
	}
}

func TestFocusIdxOpensSessionList(t *testing.T) {
	m := newModel(testGroups())
	m.projCursor = 1
	m.enterProject(1)
	if m.screen != screenSessions {
		t.Fatalf("screen = %v, want sessions", m.screen)
	}
	if m.sessions[0].ID != "root3bbb-0000" {
		t.Errorf("focused group session = %q, want root3bbb-0000", m.sessions[0].ID)
	}
}

func TestOptionsOutputDirInput(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("enter")) // open projA, select first session → options
	if m.screen != screenOptions {
		t.Fatalf("screen = %v, want options", m.screen)
	}
	// Move down to the output-directory row.
	m = send(m, key("down"), key("down"), key("down"), key("down"))
	if m.optCursor != optOutDir {
		t.Fatalf("optCursor = %d, want optOutDir (%d)", m.optCursor, optOutDir)
	}
	// Enter editing, type a path, finish with enter.
	m = send(m, key("enter"))
	if !m.editing {
		t.Fatal("expected editing mode after enter on output-dir row")
	}
	for _, ch := range []string{"o", "u", "t", "/", "r", "u", "n"} {
		m = send(m, key(ch))
	}
	m = send(m, key("enter"))
	if m.editing {
		t.Fatal("still editing after enter")
	}
	if m.outDir != "out/run" {
		t.Fatalf("outDir = %q, want out/run", m.outDir)
	}
	// Confirm from Generate; the selection carries the override.
	m = send(m, key("down"))
	if m.optCursor != optGenerate {
		t.Fatalf("optCursor = %d, want optGenerate (%d)", m.optCursor, optGenerate)
	}
	m = send(m, key("enter"))
	if m.sel.OutDir != "out/run" {
		t.Fatalf("sel.OutDir = %q, want out/run", m.sel.OutDir)
	}
}

func TestEditingModeTakesLiteralKeys(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("enter")) // → options
	m = send(m, key("down"), key("down"), key("down"), key("down"), key("enter"))
	// 'q' would normally quit; while editing it must be a literal character.
	m = send(m, key("q"), key("a"))
	if m.sel.Canceled {
		t.Fatal("'q' quit while editing; should be literal input")
	}
	if m.outDir != "qa" {
		t.Fatalf("outDir = %q, want qa", m.outDir)
	}
}

func TestOptionsUsageToggle(t *testing.T) {
	m := newModel(testGroups())
	m = send(m, key("enter"), key("enter")) // open project, select session -> options
	// move to the usage toggle row and toggle it on
	m = send(m, key("down"), key("down"), key("down"))
	if m.optCursor != optUsage {
		t.Fatalf("optCursor = %d, want optUsage (%d)", m.optCursor, optUsage)
	}
	m = send(m, key(" "))
	if !m.sel.Usage {
		t.Fatal("space on usage row should enable Selection.Usage")
	}
}

func TestClampScroll(t *testing.T) {
	// cursor below window scrolls down
	if got := clampScroll(12, 0, 10, 30); got != 3 {
		t.Errorf("clampScroll down = %d, want 3", got)
	}
	// cursor above window scrolls up
	if got := clampScroll(2, 5, 10, 30); got != 2 {
		t.Errorf("clampScroll up = %d, want 2", got)
	}
	// list shorter than window pins to 0
	if got := clampScroll(0, 4, 10, 3); got != 0 {
		t.Errorf("clampScroll short = %d, want 0", got)
	}
}
