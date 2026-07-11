package transcript

import (
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

func TestParseBasicTimeline(t *testing.T) {
	s, err := ParseFile("testdata/basic.jsonl", Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Say hello" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.SkippedLines != 1 {
		t.Errorf("SkippedLines = %d, want 1", s.SkippedLines)
	}
	if s.GitBranch != "main" {
		t.Errorf("GitBranch = %q", s.GitBranch)
	}
	// Expect: user turn, assistant turn (thinking+text+tool_use), assistant turn (Edit tool)
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(s.Turns))
	}
	if s.Turns[0].Kind != model.TurnUser {
		t.Errorf("turn0 kind = %q", s.Turns[0].Kind)
	}
	a := s.Turns[1]
	if len(a.Blocks) != 3 {
		t.Fatalf("assistant turn has %d blocks, want 3", len(a.Blocks))
	}
	tool := a.Blocks[2].Tool
	if tool == nil || tool.Name != "Bash" {
		t.Fatalf("expected Bash tool, got %+v", tool)
	}
	if tool.Result == nil || tool.Result.Content != "hi" {
		t.Errorf("Bash result = %+v", tool.Result)
	}
	if tool.Summary != "echo hi" {
		t.Errorf("Bash summary = %q", tool.Summary)
	}
	// Edit tool should have a diff.
	edit := s.Turns[2].Blocks[0].Tool
	if edit.Diff == nil || edit.Diff.Path != "/tmp/x.txt" || edit.Diff.NewText != "b" {
		t.Errorf("Edit diff = %+v", edit.Diff)
	}
}

func TestParseSubagentAttachment(t *testing.T) {
	s, err := ParseFile("testdata/subagent.jsonl", Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	task := s.Turns[1].Blocks[0].Tool
	if task.Name != "Task" {
		t.Fatalf("expected Task tool, got %q", task.Name)
	}
	if len(task.Subagents) != 1 {
		t.Fatalf("got %d subagents, want 1", len(task.Subagents))
	}
	sub := task.Subagents[0]
	if sub.Description != "research topic" {
		t.Errorf("subagent description = %q", sub.Description)
	}
	if len(sub.Turns) != 2 {
		t.Errorf("subagent turns = %d, want 2", len(sub.Turns))
	}
}

func TestParseExcludeSubagents(t *testing.T) {
	s, err := ParseFile("testdata/subagent.jsonl", Options{IncludeSubagents: false})
	if err != nil {
		t.Fatal(err)
	}
	task := s.Turns[1].Blocks[0].Tool
	if len(task.Subagents) != 0 {
		t.Errorf("expected no subagents, got %d", len(task.Subagents))
	}
}
