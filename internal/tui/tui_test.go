package tui

import (
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

func TestFlattenRowsBuildsHeadersAndSessions(t *testing.T) {
	groups := []discovery.ProjectGroup{
		{
			ProjectPath: "/tmp/projA",
			Sessions: []discovery.SessionInfo{
				{ID: "s1", Title: "First"},
				{ID: "s2", Title: "Second"},
			},
		},
		{
			ProjectPath: "/tmp/projB",
			Sessions:    []discovery.SessionInfo{{ID: "s3", Title: "Third"}},
		},
	}
	rows := flattenRows(groups)
	// 2 headers + 3 sessions
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5", len(rows))
	}
	if !rows[0].IsHeader || rows[0].Label != "/tmp/projA" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1].IsHeader || rows[1].Session.ID != "s1" {
		t.Errorf("row1 = %+v", rows[1])
	}
}

func TestFirstSelectableRow(t *testing.T) {
	rows := []Row{{IsHeader: true}, {IsHeader: false, Session: discovery.SessionInfo{ID: "x"}}}
	if got := firstSelectable(rows); got != 1 {
		t.Errorf("firstSelectable = %d, want 1", got)
	}
}
