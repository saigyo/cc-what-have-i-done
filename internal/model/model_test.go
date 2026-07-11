package model

import (
	"testing"
	"time"
)

func TestSessionDuration(t *testing.T) {
	s := Session{
		StartedAt: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC),
		EndedAt:   time.Date(2026, 7, 11, 10, 5, 0, 0, time.UTC),
	}
	if got := s.Duration(); got != 5*time.Minute {
		t.Errorf("Duration() = %v, want 5m", got)
	}
}

func TestDisplayTitleFallback(t *testing.T) {
	if got := (Session{}).DisplayTitle(); got != "Untitled session" {
		t.Errorf("DisplayTitle() = %q, want fallback", got)
	}
	if got := (Session{Title: "Fix parser"}).DisplayTitle(); got != "Fix parser" {
		t.Errorf("DisplayTitle() = %q, want title", got)
	}
}
