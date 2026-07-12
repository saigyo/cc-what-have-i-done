package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
	"github.com/saigyo/cc-what-have-i-done/internal/redact"
	"github.com/saigyo/cc-what-have-i-done/internal/render"
	"github.com/saigyo/cc-what-have-i-done/internal/transcript"
)

func TestEndToEndRedactedReport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s.jsonl")
	lines := strings.Join([]string{
		`{"type":"ai-title","aiTitle":"E2E"}`,
		`{"type":"user","message":{"role":"user","content":"my key is AKIAIOSFODNN7EXAMPLE"},"timestamp":"2026-07-11T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-07-11T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"a.txt"}]},"timestamp":"2026-07-11T10:00:02Z"}`,
	}, "\n")
	if err := os.WriteFile(src, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := transcript.ParseFile(src, transcript.Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	redact.Session(&sess, redact.Config{})
	out := filepath.Join(dir, "report")
	if err := render.Site(sess, out, render.Options{}); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("secret leaked into report")
	}
	if !strings.Contains(s, "REDACTED:aws-key") {
		t.Error("expected redaction marker in report")
	}
	if !strings.Contains(s, "a.txt") {
		t.Error("tool result missing from report")
	}
}

func TestSiteWritesSubagentPagesAndLinks(t *testing.T) {
	agent := model.AgentSession{
		ID:          "abc123",
		Description: "Implement Task 12",
		AgentType:   "general-purpose",
		ToolUseID:   "toolu_9",
		Session: model.Session{Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "brief"}}},
			{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockText, Text: "agent work"}}},
		}},
	}
	s := model.Session{
		Title: "root",
		Turns: []model.Turn{
			{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
				ID: "toolu_9", Name: "Agent", Summary: "Implement Task 12",
			}}}},
			{Kind: model.TurnAgentResult, AgentID: "abc123", AgentStatus: "completed",
				AgentSummary: `Agent "Implement Task 12" finished`,
				Blocks:       []model.Block{{Type: model.BlockText, Text: "done"}}},
		},
		Agents: []model.AgentSession{agent},
	}
	dir := t.TempDir()
	if err := render.Site(s, dir, render.Options{}); err != nil {
		t.Fatal(err)
	}

	page, err := os.ReadFile(filepath.Join(dir, "subagents", "agent-abc123.html"))
	if err != nil {
		t.Fatalf("agent page missing: %v", err)
	}
	for _, want := range []string{"agent work", `href="../index.html"`, `href="../assets/styles.css"`, "Implement Task 12"} {
		if !strings.Contains(string(page), want) {
			t.Errorf("agent page missing %q", want)
		}
	}

	index, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Both the tool card and the result card link to the agent page.
	if got := strings.Count(string(index), `href="subagents/agent-abc123.html"`); got < 2 {
		t.Errorf("index has %d links to the agent page, want >= 2", got)
	}
}

func TestSiteNoAgentsWritesNoSubagentsDir(t *testing.T) {
	dir := t.TempDir()
	if err := render.Site(model.Session{Title: "x"}, dir, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subagents")); !os.IsNotExist(err) {
		t.Errorf("subagents dir must not exist for agent-less sessions (err=%v)", err)
	}
}
