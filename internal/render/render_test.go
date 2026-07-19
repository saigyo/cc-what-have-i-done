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

func TestAgentToolCardRendersPromptAndResultAsMarkdown(t *testing.T) {
	tc := &model.ToolCall{
		Name:        "Agent",
		Summary:     "Implement Task 1: scaffold",
		AgentPrompt: "# Do the thing\n\nWith **emphasis**.",
		InputJSON:   `{"prompt":"# Do the thing"}`,
		Result:      &model.ToolResult{Content: "**Status:** DONE"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))

	if !strings.Contains(out, `class="agent-prompt"`) {
		t.Errorf("expected agent-prompt block, got %q", out)
	}
	if !strings.Contains(out, "<strong>emphasis</strong>") {
		t.Errorf("prompt should be rendered as markdown: %q", out)
	}
	if !strings.Contains(out, `class="agent-result-body"`) {
		t.Errorf("expected agent-result-body block: %q", out)
	}
	if !strings.Contains(out, "<strong>Status:</strong>") {
		t.Errorf("result should be rendered as markdown: %q", out)
	}
	if strings.Contains(out, `class="tool-input"`) {
		t.Errorf("agent card must not dump raw JSON input: %q", out)
	}
	if strings.Contains(out, `class="tool-result"`) {
		t.Errorf("agent result must not use the monospace tool-result block: %q", out)
	}
	if !strings.Contains(out, "Implement Task 1: scaffold") {
		t.Errorf("tool header should show the agent description: %q", out)
	}
}

func TestNonAgentToolResultStaysMonospace(t *testing.T) {
	tc := &model.ToolCall{
		Name:      "Bash",
		Summary:   "ls",
		InputJSON: `{"command":"ls"}`,
		Result:    &model.ToolResult{Content: "file1\nfile2"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result"`) {
		t.Errorf("non-agent result should keep the monospace tool-result block: %q", out)
	}
	if strings.Contains(out, `class="agent-result-body"`) {
		t.Errorf("non-agent result must not be markdown-rendered: %q", out)
	}
}

func TestAskUserQuestionCardRendersOptionsAndAnswers(t *testing.T) {
	tc := &model.ToolCall{
		Name:    "AskUserQuestion",
		Summary: "Language",
		Questions: []model.Question{{
			Header: "Language",
			Prompt: "Go or explore an alternative?",
			Options: []model.QuestionOption{
				{Label: "Go (Recommended)", Description: "Single static **binary**."},
				{Label: "Rust", Description: "Also single-binary.", Preview: "$ cargo build"},
			},
		}},
		InputJSON: `{"questions":[...]}`,
		Result:    &model.ToolResult{Content: `Your questions have been answered: "Go or explore an alternative?"="Go (Recommended)". You can now continue.`},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))

	if !strings.Contains(out, `class="ask-header"`) || !strings.Contains(out, ">Language<") {
		t.Errorf("expected header chip with the question header: %q", out)
	}
	if !strings.Contains(out, "<strong>binary</strong>") {
		t.Errorf("option description should render as markdown: %q", out)
	}
	if !strings.Contains(out, `class="ask-option-preview"`) || !strings.Contains(out, "cargo build") {
		t.Errorf("expected preview block for the option that has one: %q", out)
	}
	if !strings.Contains(out, `class="ask-option ask-option-selected"`) {
		t.Errorf("the chosen option should be marked selected: %q", out)
	}
	if !strings.Contains(out, `class="ask-answers"`) || !strings.Contains(out, `class="ask-answers-a">Go (Recommended)<`) {
		t.Errorf("expected an answers summary with the chosen answer: %q", out)
	}
	if strings.Contains(out, `class="tool-input"`) {
		t.Errorf("AskUserQuestion card must not dump raw JSON input: %q", out)
	}
	if strings.Contains(out, "questions have been answered") {
		t.Errorf("raw result sentence should be replaced by the answers summary: %q", out)
	}
}

func TestAskUserQuestionSelectionIsScopedPerQuestion(t *testing.T) {
	// Two questions share the option labels "Yes"/"No". The result picks "Yes"
	// for the first and "No" for the second; each question must mark only its
	// own answer, not the label shared with the other question.
	tc := &model.ToolCall{
		Name: "AskUserQuestion",
		Questions: []model.Question{
			{Header: "Cache", Prompt: "Enable cache?", Options: []model.QuestionOption{{Label: "Yes"}, {Label: "No"}}},
			{Header: "Verbose", Prompt: "Verbose logs?", Options: []model.QuestionOption{{Label: "Yes"}, {Label: "No"}}},
		},
		Result: &model.ToolResult{Content: `Your questions have been answered: "Enable cache?"="Yes", "Verbose logs?"="No". Continue.`},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))

	// Exactly two options selected across the whole card (one per question), not
	// four (which the old whole-result scan would have produced).
	if n := strings.Count(out, "ask-option-selected"); n != 2 {
		t.Errorf("expected exactly 2 selected options (one per question), got %d: %q", n, out)
	}
}

func TestAskUserQuestionFallsBackToRawResultWhenUnparsed(t *testing.T) {
	// A result whose prompt does not match the question falls back to raw text.
	tc := &model.ToolCall{
		Name: "AskUserQuestion",
		Questions: []model.Question{{
			Header:  "Scope",
			Prompt:  "What scope?",
			Options: []model.QuestionOption{{Label: "Small"}},
		}},
		Result: &model.ToolResult{Content: "some unexpected result text"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result"`) || !strings.Contains(out, "some unexpected result text") {
		t.Errorf("unparseable result should fall back to the raw monospace block: %q", out)
	}
}

func TestAskUserQuestionErrorFallbackKeepsErrorAffordance(t *testing.T) {
	// An errored result that also can't be parsed must keep the error affordance.
	tc := &model.ToolCall{
		Name:      "AskUserQuestion",
		Questions: []model.Question{{Header: "Scope", Prompt: "What scope?", Options: []model.QuestionOption{{Label: "Small"}}}},
		Result:    &model.ToolResult{Content: "interrupted by user", IsError: true},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result tool-result-error"`) {
		t.Errorf("errored AskUserQuestion fallback should keep the tool-result-error affordance: %q", out)
	}
}

func TestAgentErrorResultKeepsErrorAffordance(t *testing.T) {
	tc := &model.ToolCall{
		Name:   "Agent",
		Result: &model.ToolResult{Content: "**Status:** BLOCKED", IsError: true},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="agent-result-body tool-result-error"`) {
		t.Errorf("agent error result should keep the tool-result-error affordance: %q", out)
	}
}

func TestTaskCreateCardTitleDescriptionAndResultSuppression(t *testing.T) {
	tc := &model.ToolCall{
		Name:        "TaskCreate",
		Summary:     "Ship the feature",
		TaskNumber:  "12",
		Description: "First **this**, then that.",
		InputJSON:   `{"subject":"Ship the feature"}`,
		Result:      &model.ToolResult{Content: "Task #12 created successfully: Ship the feature"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))

	if !strings.Contains(out, "#12 · Ship the feature") {
		t.Errorf("header should show the task number and subject: %q", out)
	}
	if !strings.Contains(out, `class="task-desc"`) {
		t.Errorf("expected task-desc block: %q", out)
	}
	if !strings.Contains(out, "<strong>this</strong>") {
		t.Errorf("description should be rendered as markdown: %q", out)
	}
	if strings.Contains(out, `class="tool-input"`) {
		t.Errorf("TaskCreate card must not dump raw JSON input: %q", out)
	}
	if strings.Contains(out, "tool-result") {
		t.Errorf("redundant TaskCreate result must be suppressed: %q", out)
	}
}

func TestTaskCreateWithoutResultShowsSubjectOnly(t *testing.T) {
	tc := &model.ToolCall{Name: "TaskCreate", Summary: "Ship the feature", Description: "Steps."}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if strings.Contains(out, "#") && strings.Contains(out, "· Ship the feature") {
		t.Errorf("no result: header must not carry a number prefix: %q", out)
	}
	if !strings.Contains(out, "Ship the feature") {
		t.Errorf("header should show the subject: %q", out)
	}
}

func TestTaskCreateErrorResultStillShown(t *testing.T) {
	tc := &model.ToolCall{
		Name:    "TaskCreate",
		Summary: "Ship the feature",
		Result:  &model.ToolResult{Content: "boom", IsError: true},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result tool-result-error"`) {
		t.Errorf("error result must keep the error affordance: %q", out)
	}
}

func TestTaskUpdateCardTitleAndResultSuppression(t *testing.T) {
	tc := &model.ToolCall{
		Name:      "TaskUpdate",
		Summary:   "#3 · completed",
		InputJSON: `{"taskId":"3","status":"completed"}`,
		Result:    &model.ToolResult{Content: "Updated task #3 status"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, "#3 · completed") {
		t.Errorf("header should show id and status: %q", out)
	}
	if !strings.Contains(out, `class="tool-input"`) {
		t.Errorf("TaskUpdate body keeps the small input JSON: %q", out)
	}
	if strings.Contains(out, "tool-result") {
		t.Errorf("redundant TaskUpdate result must be suppressed: %q", out)
	}
}

func TestTaskUpdateErrorResultStillShown(t *testing.T) {
	tc := &model.ToolCall{
		Name:    "TaskUpdate",
		Summary: "#3 · completed",
		Result:  &model.ToolResult{Content: "no such task", IsError: true},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result tool-result-error"`) {
		t.Errorf("error result must keep the error affordance: %q", out)
	}
}

func TestHeaderSummaryPrefixesOnlyTaskCreate(t *testing.T) {
	tc := &model.ToolCall{Name: "Bash", Summary: "ls", TaskNumber: "9"}
	if got := headerSummary(tc); got != "ls" {
		t.Errorf("headerSummary = %q; only TaskCreate cards get a number prefix", got)
	}
}

func TestTaskCreateUnexpectedResultTextStillShown(t *testing.T) {
	tc := &model.ToolCall{
		Name:       "TaskCreate",
		Summary:    "Ship the feature",
		TaskNumber: "12",
		Result:     &model.ToolResult{Content: "Task #12 created with warnings: dependency cycle"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result"`) {
		t.Errorf("a result that is not the plain success line must stay visible: %q", out)
	}
}

func TestTaskCreateMultilineResultStillShown(t *testing.T) {
	tc := &model.ToolCall{
		Name:       "TaskCreate",
		Summary:    "Ship the feature",
		TaskNumber: "12",
		Result:     &model.ToolResult{Content: "Task #12 created successfully: Ship the feature\nNote: blocked by task #11"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result"`) {
		t.Errorf("a multi-line result carries extra detail and must stay visible: %q", out)
	}
}

func TestTaskUpdateUnexpectedResultTextStillShown(t *testing.T) {
	tc := &model.ToolCall{
		Name:    "TaskUpdate",
		Summary: "#3 · completed",
		Result:  &model.ToolResult{Content: "Task #3 is blocked by task #2"},
	}
	turn := model.Turn{Kind: model.TurnAssistant, Blocks: []model.Block{{Type: model.BlockToolUse, Tool: tc}}}
	out := string(renderTurnBody(turn, newAgentLinks(nil, "")))
	if !strings.Contains(out, `class="tool-result"`) {
		t.Errorf("a result that is not the plain status line must stay visible: %q", out)
	}
}

func TestSiteShowsReleaseVersionLink(t *testing.T) {
	dir := t.TempDir()
	if err := Site(sampleSession(), dir, Options{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, `href="https://github.com/saigyo/cc-what-have-i-done/releases/tag/v1.2.3"`) {
		t.Errorf("index.html missing release link")
	}
	if !strings.Contains(s, `class="brand-version"`) || !strings.Contains(s, ">v1.2.3</a>") {
		t.Errorf("index.html missing version label")
	}
	if !strings.Contains(s, `>ccwhid <a`) {
		t.Errorf("brand text and version link must stay separate text nodes")
	}
}

func TestSiteShowsDevBuildLink(t *testing.T) {
	dir := t.TempDir()
	if err := Site(sampleSession(), dir, Options{Version: "dev"}); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, `href="https://github.com/saigyo/cc-what-have-i-done/"`) {
		t.Errorf("index.html missing repo link")
	}
	if !strings.Contains(s, ">dev build</a>") {
		t.Errorf("index.html missing dev build label")
	}
}
