package transcript

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

func TestParseDeduplicatesUsagePerMessageID(t *testing.T) {
	// Claude Code writes one assistant message as several records (one per
	// content block), each repeating the same usage. Usage must be counted
	// once per message id, not once per record.
	mkRec := func(block string) string {
		return `{"type":"assistant","message":{"role":"assistant","id":"msg_abc","model":"claude-opus-4-8","content":[` +
			block + `],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":500,"cache_creation_input_tokens":30}},"timestamp":"2026-07-12T10:00:00Z"}`
	}
	lines := strings.Join([]string{
		mkRec(`{"type":"thinking","thinking":"hmm"}`),
		mkRec(`{"type":"text","text":"hello"}`),
		mkRec(`{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}`),
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Three turns are produced (one per record), but exactly one carries usage.
	withUsage := 0
	for _, turn := range s.Turns {
		if turn.Usage != nil {
			withUsage++
		}
	}
	if withUsage != 1 {
		t.Fatalf("usage attached to %d turns, want 1 (deduped by message id)", withUsage)
	}
	// The one usage-bearing turn holds the message's full (single-counted) usage.
	for _, turn := range s.Turns {
		if turn.Usage != nil && (turn.Usage.Input != 100 || turn.Usage.Output != 20 || turn.Usage.CacheRead != 500) {
			t.Errorf("deduped usage = %+v, want input100/output20/cacheRead500", *turn.Usage)
		}
	}
}

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

func TestParseSubagentInnerToolResult(t *testing.T) {
	s, err := ParseFile("testdata/subagent_inner_tool.jsonl", Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	task := s.Turns[1].Blocks[0].Tool
	if len(task.Subagents) != 1 {
		t.Fatalf("got %d subagents, want 1", len(task.Subagents))
	}
	var inner *model.ToolCall
	for _, tn := range task.Subagents[0].Turns {
		for _, bl := range tn.Blocks {
			if bl.Tool != nil && bl.Tool.Name == "Bash" {
				inner = bl.Tool
			}
		}
	}
	if inner == nil {
		t.Fatal("inner Bash tool not found in subagent turns")
	}
	if inner.Result == nil || inner.Result.Content != "file.txt" {
		t.Errorf("inner tool result = %+v, want content 'file.txt'", inner.Result)
	}
}

func TestParseAttachesModelAndUsage(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"hi"},"timestamp":"2026-07-12T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":100,"output_tokens":20,"cache_read_input_tokens":500,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}},"timestamp":"2026-07-12T10:00:01Z"}`,
		`{"type":"assistant","message":{"role":"assistant","model":"claude-opus-4-8","content":[{"type":"text","text":"no usage here"}]},"timestamp":"2026-07-12T10:00:02Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	// turns: [user, assistant-with-usage, assistant-without-usage]
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(s.Turns))
	}
	if s.Turns[0].Usage != nil {
		t.Error("user turn should have nil usage")
	}
	a := s.Turns[1]
	if a.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q", a.Model)
	}
	if a.Usage == nil {
		t.Fatal("assistant usage is nil")
	}
	want := model.Usage{Input: 100, Output: 20, CacheRead: 500, CacheWrite5m: 10, CacheWrite1h: 20}
	if *a.Usage != want {
		t.Errorf("Usage = %+v, want %+v", *a.Usage, want)
	}
	if s.Turns[2].Usage != nil {
		t.Error("assistant without usage should have nil Usage")
	}
}

func TestParseAggregateCacheCreationDefaultsTo5m(t *testing.T) {
	// Only the aggregate cache_creation_input_tokens is present (no split).
	line := `{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-5","content":[{"type":"text","text":"x"}],"usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":40}},"timestamp":"2026-07-12T10:00:00Z"}`
	s, err := Parse(strings.NewReader(line), Options{})
	if err != nil {
		t.Fatal(err)
	}
	u := s.Turns[0].Usage
	if u == nil || u.CacheWrite5m != 40 || u.CacheWrite1h != 0 {
		t.Errorf("aggregate cache_creation should default to 5m: %+v", u)
	}
}

func TestBuildToolCallAgentSummaryAndPrompt(t *testing.T) {
	b := apiBlock{
		Type:  "tool_use",
		ID:    "t1",
		Name:  "Agent",
		Input: json.RawMessage(`{"description":"Implement Task 1","subagent_type":"general-purpose","prompt":"# Do it\n\nWith **care**."}`),
	}
	tc := buildToolCall(b)
	if tc.Summary != "Implement Task 1" {
		t.Errorf("Summary = %q, want the agent description", tc.Summary)
	}
	if tc.AgentPrompt != "# Do it\n\nWith **care**." {
		t.Errorf("AgentPrompt = %q", tc.AgentPrompt)
	}
	if !tc.IsAgent() {
		t.Error("Agent tool should report IsAgent")
	}
}

func TestBuildToolCallSkillSummaryIsSkillName(t *testing.T) {
	b := apiBlock{
		Type:  "tool_use",
		ID:    "t2",
		Name:  "Skill",
		Input: json.RawMessage(`{"skill":"brainstorming","args":"an idea"}`),
	}
	tc := buildToolCall(b)
	if tc.Summary != "brainstorming" {
		t.Errorf("Summary = %q, want the skill name", tc.Summary)
	}
	if tc.AgentPrompt != "" {
		t.Errorf("Skill call should not set AgentPrompt, got %q", tc.AgentPrompt)
	}
}
