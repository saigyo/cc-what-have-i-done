package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
	"github.com/saigyo/cc-what-have-i-done/internal/usage"
)

func TestMarkdownRendersHeadingAndCode(t *testing.T) {
	out := string(Markdown("# Title\n\n```go\nfmt.Println(\"hi\")\n```"))
	if !strings.Contains(out, "<h1") {
		t.Errorf("expected <h1> in %q", out)
	}
	if !strings.Contains(out, "Println") {
		t.Errorf("expected highlighted code in output")
	}
	if !strings.Contains(out, "<span") {
		t.Errorf("expected chroma-highlighted <span> tokens in code output, got %q", out)
	}
}

func TestDiffHTMLMarksLines(t *testing.T) {
	out := string(DiffHTML(&model.Diff{Path: "x.txt", OldText: "a", NewText: "b"}))
	if !strings.Contains(out, "diff-del") || !strings.Contains(out, "diff-add") {
		t.Errorf("diff HTML missing add/del classes: %q", out)
	}
}

func TestStripANSI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"\x1b[1mOpus 4.8\x1b[22m", "Opus 4.8"},   // SGR bold on/off
		{"\x1b[0;31mred\x1b[0m text", "red text"}, // colour codes
		{"plain text", "plain text"},              // fast path, unchanged
		{"array[1m] index", "array[1m] index"},    // literal, no ESC → kept
		{"\x1b]0;title\x07done", "done"},          // OSC + BEL
		{"a\x1b[Kb", "ab"},                        // erase-line CSI
	}
	for _, c := range cases {
		if got := StripANSI(c.in); got != c.want {
			t.Errorf("StripANSI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderStripsANSIFromReport(t *testing.T) {
	s := model.Session{
		ID:    "x",
		Title: "T",
		Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "set model to \x1b[1mOpus\x1b[22m now"}}},
			{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
				Name: "Bash", Summary: "echo hi", Result: &model.ToolResult{Content: "\x1b[32mgreen\x1b[0m output"},
			}}}},
		},
	}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	body := string(html)
	if strings.ContainsRune(body, 0x1b) {
		t.Error("report still contains a raw ESC byte")
	}
	for _, want := range []string{"set model to Opus now", "green output"} {
		if !strings.Contains(body, want) {
			t.Errorf("report missing cleaned text %q", want)
		}
	}
	if strings.Contains(body, "[1m") || strings.Contains(body, "[22m") || strings.Contains(body, "[32m") {
		t.Error("report still shows leaked ANSI parameter text")
	}
}

func sampleSession() model.Session {
	return model.Session{
		ID:          "abcd1234",
		ProjectPath: "/tmp/proj",
		Title:       "Sample",
		StartedAt:   time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC),
		EndedAt:     time.Date(2026, 7, 11, 10, 1, 0, 0, time.UTC),
		Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "do it"}}},
			{Kind: model.TurnAssistant, Blocks: []model.Block{
				{Type: model.BlockText, Text: "on it"},
				{Type: model.BlockToolUse, Tool: &model.ToolCall{
					Name: "Bash", Summary: "ls", Result: &model.ToolResult{Content: "file.txt"},
				}},
			}},
		},
	}
}

func TestPreviewRuneSafe(t *testing.T) {
	s := strings.Repeat("ä", 80) // 80 two-byte runes, exceeds n=60
	got := preview(s, 60)
	if !utf8.ValidString(got) {
		t.Errorf("preview produced invalid UTF-8: %q", got)
	}
	if []rune(got)[len([]rune(got))-1] != '…' {
		t.Errorf("preview should end with ellipsis, got %q", got)
	}
}

func TestSiteRendersUsageWhenEnabled(t *testing.T) {
	s := model.Session{
		ID: "x", Title: "T",
		Turns: []model.Turn{
			{Kind: model.TurnUser, Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
			{Kind: model.TurnAssistant, Model: "claude-opus-4-8",
				Usage:  &model.Usage{Input: 1000, Output: 200},
				Blocks: []model.Block{{Type: model.BlockText, Text: "ok"}}},
		},
	}
	dir := t.TempDir()
	if err := Site(s, dir, Options{Usage: true}); err != nil {
		t.Fatal(err)
	}
	html := readIndex(t, dir)
	for _, want := range []string{
		"usage-card", "usage-badge", "in+out", "prices as of", "claude-opus-4-8",
		// per-model table carries the full breakdown + a Total row
		"<th>input</th>", "<th>output</th>", "<th>cache read</th>", "<th>cache write</th>",
		"usage-total", "Total",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("usage output missing %q", want)
		}
	}
}

func TestSiteDropsZeroTokenModelRow(t *testing.T) {
	s := model.Session{
		ID: "x", Title: "T",
		Turns: []model.Turn{
			{Kind: model.TurnAssistant, Model: "claude-opus-4-8",
				Usage:  &model.Usage{Input: 1000, Output: 200},
				Blocks: []model.Block{{Type: model.BlockText, Text: "ok"}}},
			{Kind: model.TurnAssistant, Model: "<synthetic>",
				Usage:  &model.Usage{}, // all-zero
				Blocks: []model.Block{{Type: model.BlockText, Text: "x"}}},
		},
	}
	dir := t.TempDir()
	if err := Site(s, dir, Options{Usage: true}); err != nil {
		t.Fatal(err)
	}
	html := readIndex(t, dir)
	if strings.Contains(html, "synthetic") {
		t.Error("all-zero <synthetic> row should be dropped from the usage table")
	}
	if strings.Contains(html, "unpriced models") {
		t.Error("footnote should not mention unpriced models when the only unpriced model was dropped")
	}
	if !strings.Contains(html, "sub-agent sessions") {
		t.Error("footnote should note sub-agent sessions are excluded")
	}
}

func TestSiteOmitsUsageByDefault(t *testing.T) {
	s := model.Session{ID: "x", Title: "T", Turns: []model.Turn{
		{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: &model.Usage{Input: 1000, Output: 200},
			Blocks: []model.Block{{Type: model.BlockText, Text: "ok"}}},
	}}
	dir := t.TempDir()
	if err := Site(s, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readIndex(t, dir), "usage-card") {
		t.Error("usage card should be absent without Options.Usage")
	}
}

// readIndex reads the generated index.html.
func readIndex(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSiteWritesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := Site(sampleSession(), dir, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", "assets/styles.css", "assets/app.js"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	for _, want := range []string{"Sample", "do it", "on it", "Bash", "assets/styles.css", "assets/app.js"} {
		if !strings.Contains(s, want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}

func TestAgentResultTurnRendersAsAgentCard(t *testing.T) {
	s := model.Session{Turns: []model.Turn{{
		Kind:         model.TurnAgentResult,
		AgentStatus:  "completed",
		AgentSummary: `Agent "Implement Task 12: Profiles view" finished`,
		Blocks:       []model.Block{{Type: model.BlockText, Text: "All done."}},
	}}}
	d := buildViewModel(s, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""))
	tv := d.Turns[0]
	if tv.RoleLabel != "Agent · Implement Task 12: Profiles view" {
		t.Errorf("RoleLabel = %q", tv.RoleLabel)
	}
	if tv.Status != "completed" {
		t.Errorf("Status = %q", tv.Status)
	}
	if tv.Kind != "agent-result" {
		t.Errorf("Kind = %q", tv.Kind)
	}
	if tv.RoleLabel == "You" {
		t.Error("agent result must never be attributed to the user")
	}
}

func TestAgentRoleLabelFallsBackWithoutQuotes(t *testing.T) {
	if got := agentRoleLabel("something finished"); got != "Agent" {
		t.Errorf("agentRoleLabel = %q, want Agent", got)
	}
	if got := agentRoleLabel(""); got != "Agent" {
		t.Errorf("agentRoleLabel(empty) = %q, want Agent", got)
	}
}

func TestUsageViewSubagentLineAndFootnote(t *testing.T) {
	c := 5.0
	r := usage.Report{
		HasAnyUsage:   true,
		Total:         usage.TokenCounts{Input: 2_000_000},
		Subagents:     usage.TokenCounts{Input: 1_000_000},
		SubagentsCost: &c,
		AgentSessions: 3,
		PricesAsOf:    "2026-07",
	}
	v := buildUsageView(r)
	if v.SubLine != "of which subagents: 1.0M in+out · ~$5.00 (3 sessions)" {
		t.Errorf("SubLine = %q", v.SubLine)
	}
	if !strings.Contains(v.Footnote, "Includes 3 linked subagent session(s)") {
		t.Errorf("footnote = %q", v.Footnote)
	}
	if strings.Contains(v.Footnote, "sub-agent sessions stored as separate files") {
		t.Errorf("old exclusion wording must go when agents are included: %q", v.Footnote)
	}
}

func TestUsageViewNoSubagentLineWithoutAgents(t *testing.T) {
	r := usage.Report{HasAnyUsage: true, Total: usage.TokenCounts{Input: 100}, PricesAsOf: "2026-07"}
	v := buildUsageView(r)
	if v.SubLine != "" {
		t.Errorf("SubLine = %q, want empty", v.SubLine)
	}
	if !strings.Contains(v.Footnote, "sub-agent sessions stored as separate files, and server-tool fees, are excluded") {
		t.Errorf("existing footnote must stay when no agents: %q", v.Footnote)
	}
}

func TestSidechainAgentResultSummaryIsEscaped(t *testing.T) {
	tc := &model.ToolCall{Name: "Task", Subagents: []model.Subagent{{
		Description: "d",
		Turns: []model.Turn{{
			Kind:         model.TurnAgentResult,
			AgentSummary: `Agent "<img src=x onerror=alert(1)>" finished`,
			Blocks:       []model.Block{{Type: model.BlockText, Text: "x"}},
		}},
	}}}
	out := renderTool(tc, newAgentLinks(nil, ""))
	if strings.Contains(out, "<img") {
		t.Fatal("agent summary must be HTML-escaped in sidechain rendering")
	}
	if !strings.Contains(out, "&lt;img") {
		t.Error("escaped summary not found in output")
	}
}
