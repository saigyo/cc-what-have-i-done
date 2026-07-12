package redact

import (
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

func TestRedactPatterns(t *testing.T) {
	r := New("/Users/markus")
	cases := []struct {
		in       string
		wantKind string
	}{
		{"key AKIAIOSFODNN7EXAMPLE here", "aws-key"},
		{"token sk-abcdefghijklmnopqrstuvwxy012345678", "token"},
		{"ghp_0123456789abcdefghijklmnopqrstuvwxyz", "token"},
		{"export API_KEY=supersecretvalue123", "assignment"},
	}
	for _, c := range cases {
		got := r.String(c.in)
		if !strings.Contains(got, "[REDACTED:"+c.wantKind+"]") {
			t.Errorf("String(%q) = %q, want kind %s", c.in, got, c.wantKind)
		}
	}
}

func TestRedactHomeDir(t *testing.T) {
	r := New("/Users/markus")
	if got := r.String("/Users/markus/secret/file"); got != "~/secret/file" {
		t.Errorf("home rewrite = %q", got)
	}
}

func TestRedactSessionWalksBlocks(t *testing.T) {
	s := &model.Session{Turns: []model.Turn{{
		Kind: model.TurnAssistant,
		Blocks: []model.Block{
			{Type: model.BlockText, Text: "my key AKIAIOSFODNN7EXAMPLE"},
			{Type: model.BlockToolUse, Tool: &model.ToolCall{
				Name:    "Bash",
				Summary: "echo AKIAIOSFODNN7EXAMPLE",
				Result:  &model.ToolResult{Content: "AKIAIOSFODNN7EXAMPLE"},
			}},
		},
	}}}
	Session(s, "/Users/markus")
	if strings.Contains(s.Turns[0].Blocks[0].Text, "AKIA") {
		t.Error("block text not redacted")
	}
	tool := s.Turns[0].Blocks[1].Tool
	if strings.Contains(tool.Summary, "AKIA") || strings.Contains(tool.Result.Content, "AKIA") {
		t.Error("tool fields not redacted")
	}
}

func TestRedactSessionLevelFields(t *testing.T) {
	s := &model.Session{
		ProjectPath: "/Users/markus/IdeaProjects/app",
		Turns: []model.Turn{{
			Kind: model.TurnAssistant,
			Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
				Name:      "Task",
				Subagents: []model.Subagent{{Description: "run AKIAIOSFODNN7EXAMPLE"}},
			}}},
		}},
	}
	Session(s, "/Users/markus")
	if strings.Contains(s.ProjectPath, "/Users/markus") {
		t.Errorf("ProjectPath home not rewritten: %q", s.ProjectPath)
	}
	if strings.Contains(s.Turns[0].Blocks[0].Tool.Subagents[0].Description, "AKIA") {
		t.Error("subagent description not redacted")
	}
}

func TestRedactAgentPrompt(t *testing.T) {
	s := &model.Session{Turns: []model.Turn{{
		Kind: model.TurnAssistant,
		Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			Name:        "Agent",
			AgentPrompt: "read the brief at /Users/markus/IdeaProjects/app/brief.md and use AKIAIOSFODNN7EXAMPLE",
		}}},
	}}}
	Session(s, "/Users/markus")
	got := s.Turns[0].Blocks[0].Tool.AgentPrompt
	if strings.Contains(got, "/Users/markus") {
		t.Errorf("agent prompt home path not rewritten: %q", got)
	}
	if strings.Contains(got, "AKIA") {
		t.Errorf("agent prompt secret not redacted: %q", got)
	}
}
