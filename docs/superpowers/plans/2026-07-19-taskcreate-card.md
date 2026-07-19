# TaskCreate / TaskUpdate Card Rendering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render `TaskCreate` cards with `#<n> · <subject>` in the title and the description as markdown, and `TaskUpdate` cards with `#<taskId> · <status>` in the title; hide the redundant result line for both when it adds nothing.

**Architecture:** Follows the existing per-tool structured-field pattern on `model.ToolCall` (as done for `Task`/`Agent` prompts and `AskUserQuestion`). Parse time fills `Summary`/`Description`; the task number is extracted when the `tool_result` attaches; the render layer composes the title and decides what to suppress.

**Tech Stack:** Go 1.26, stdlib only (`html/template`, `strings`); tests with plain `testing`.

**Spec:** `docs/superpowers/specs/2026-07-19-taskcreate-card-design.md`

## Global Constraints

- `TaskList` / `TaskGet` stay untouched (out of scope per spec).
- `ToolCall.Summary` stays the plain parse-time text (subject for TaskCreate); the `#n ·` prefix is composed only at render time so search text is unaffected.
- TaskUpdate's summary IS parse-time (`#<taskId> · <status>`) because it comes purely from the input.
- Result suppression never applies to error results.
- No new dependencies; no regexp — use `strings` helpers like the rest of the codebase.
- Run `gofmt -l internal/` before each commit; it must print nothing.
- Every commit message ends with the two trailer lines shown in Task 1 Step 5 (Co-Authored-By + Claude-Session).

---

### Task 1: Parse-time summaries and the Description field

**Files:**
- Modify: `internal/model/model.go` (ToolCall struct ~line 77, helpers ~line 89)
- Modify: `internal/transcript/parse.go` (`buildToolCall` ~line 336, `toolSummary` ~line 411)
- Test: `internal/transcript/parse_test.go`

**Interfaces:**
- Consumes: existing `buildToolCall(b apiBlock) *model.ToolCall`, `str(m map[string]any, key string) string`, `toolSummary(name string, in map[string]any) string`.
- Produces: `model.ToolCall.Description string` (markdown, TaskCreate only), `(*model.ToolCall).IsTaskCreate() bool`, `(*model.ToolCall).IsTaskUpdate() bool`, `TaskCreate` summary = subject, `TaskUpdate` summary = `#<id> · <status>` (degrading to the present part). Task 2 and Task 3 rely on these exact names.

- [ ] **Step 1: Write the failing tests**

Append to `internal/transcript/parse_test.go`:

```go
func TestBuildToolCallTaskCreateSummaryAndDescription(t *testing.T) {
	b := apiBlock{
		Type:  "tool_use",
		ID:    "t4",
		Name:  "TaskCreate",
		Input: json.RawMessage(`{"subject":"Task 1: Parse records","description":"Do it with **care**."}`),
	}
	tc := buildToolCall(b)
	if tc.Summary != "Task 1: Parse records" {
		t.Errorf("Summary = %q, want the subject", tc.Summary)
	}
	if tc.Description != "Do it with **care**." {
		t.Errorf("Description = %q", tc.Description)
	}
	if !tc.IsTaskCreate() {
		t.Error("TaskCreate tool should report IsTaskCreate")
	}
}

func TestBuildToolCallTaskUpdateSummary(t *testing.T) {
	cases := []struct {
		name, input, want string
	}{
		{"both", `{"taskId":"7","status":"completed"}`, "#7 · completed"},
		{"id only", `{"taskId":"7"}`, "#7"},
		{"status only", `{"status":"in_progress"}`, "in_progress"},
		{"neither", `{}`, ""},
	}
	for _, c := range cases {
		tc := buildToolCall(apiBlock{Type: "tool_use", ID: "t5", Name: "TaskUpdate", Input: json.RawMessage(c.input)})
		if tc.Summary != c.want {
			t.Errorf("%s: Summary = %q, want %q", c.name, tc.Summary, c.want)
		}
		if !tc.IsTaskUpdate() {
			t.Errorf("%s: should report IsTaskUpdate", c.name)
		}
		if tc.Description != "" {
			t.Errorf("%s: TaskUpdate must not set Description, got %q", c.name, tc.Description)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'TestBuildToolCallTask' -v`
Expected: compile FAIL — `tc.Description undefined`, `tc.IsTaskCreate undefined`, `tc.IsTaskUpdate undefined`.

- [ ] **Step 3: Implement model fields and parse cases**

In `internal/model/model.go`, extend the `ToolCall` struct — add one field after `AgentPrompt`:

```go
	AgentPrompt string     // for Task/Agent calls: the subagent prompt, rendered as markdown
	Description string     // for TaskCreate calls: the task description, rendered as markdown
	Questions   []Question // set for AskUserQuestion calls
```

After the `IsAskUserQuestion` method, add:

```go
// IsTaskCreate reports whether this tool call creates a tracked task.
func (t *ToolCall) IsTaskCreate() bool {
	return t.Name == "TaskCreate"
}

// IsTaskUpdate reports whether this tool call updates a tracked task.
func (t *ToolCall) IsTaskUpdate() bool {
	return t.Name == "TaskUpdate"
}
```

In `internal/transcript/parse.go`, inside `buildToolCall`, after the `IsAskUserQuestion` block:

```go
	if tc.IsAskUserQuestion() {
		tc.Questions = parseQuestions(input)
	}
	if tc.IsTaskCreate() {
		tc.Description = str(input, "description")
	}
	return tc
```

In `toolSummary`, add two cases alongside the existing ones:

```go
	case "Task", "Agent":
		return str(in, "description")
	case "TaskCreate":
		return str(in, "subject")
	case "TaskUpdate":
		return taskUpdateSummary(in)
```

Below `askQuestionSummary`, add:

```go
// taskUpdateSummary derives the collapsed-card header for a TaskUpdate call:
// "#<taskId> · <status>", degrading to whichever part is present.
func taskUpdateSummary(in map[string]any) string {
	id, status := str(in, "taskId"), str(in, "status")
	switch {
	case id != "" && status != "":
		return "#" + id + " · " + status
	case id != "":
		return "#" + id
	default:
		return status
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transcript/ ./internal/model/ -v`
Expected: all PASS (new tests plus the existing suite).

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/   # must print nothing
git add internal/model/model.go internal/transcript/parse.go internal/transcript/parse_test.go
git commit -m "feat(parse): summaries for TaskCreate/TaskUpdate cards

TaskCreate cards summarize as the task subject and carry the markdown
description in a new ToolCall.Description field; TaskUpdate cards
summarize as '#<taskId> · <status>' straight from the input.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 2: Task number extraction when the result attaches

**Files:**
- Modify: `internal/model/model.go` (ToolCall struct)
- Modify: `internal/transcript/parse.go` (`buildTurn` tool_result branch ~line 295; new helper near `taskDescription` ~line 476)
- Test: `internal/transcript/parse_test.go`

**Interfaces:**
- Consumes: `(*model.ToolCall).IsTaskCreate()` from Task 1; existing `toolIndex` result attachment in `buildTurn`.
- Produces: `model.ToolCall.TaskNumber string` (e.g. `"12"`, empty when unknown) and unexported `taskNumber(content string) string`. Task 3 relies on `TaskNumber`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/transcript/parse_test.go`:

```go
func TestTaskNumber(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Task #12 created successfully: Ship it", "12"},
		{"Task #7 created successfully", "7"},
		{"Created task 12", ""},
		{"Task #x created", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := taskNumber(c.in); got != c.want {
			t.Errorf("taskNumber(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseTaskCreateExtractsTaskNumber(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tc1","name":"TaskCreate","input":{"subject":"Ship it","description":"Steps."}}]},"timestamp":"2026-07-19T10:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tc1","content":"Task #12 created successfully: Ship it"}]},"timestamp":"2026-07-19T10:00:01Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tool := s.Turns[0].Blocks[0].Tool
	if tool.TaskNumber != "12" {
		t.Errorf("TaskNumber = %q, want \"12\"", tool.TaskNumber)
	}
}

func TestParseTaskCreateErrorResultLeavesNumberEmpty(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"tc2","name":"TaskCreate","input":{"subject":"Ship it"}}]},"timestamp":"2026-07-19T10:00:00Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tc2","content":"Task #12 created successfully: Ship it","is_error":true}]},"timestamp":"2026-07-19T10:00:01Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	tool := s.Turns[0].Blocks[0].Tool
	if tool.TaskNumber != "" {
		t.Errorf("TaskNumber = %q, want empty for an error result", tool.TaskNumber)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'TestTaskNumber|TestParseTaskCreate' -v`
Expected: compile FAIL — `undefined: taskNumber`, `tool.TaskNumber undefined`.

- [ ] **Step 3: Implement the field, the extractor, and the hook**

In `internal/model/model.go`, add to `ToolCall` after `Description`:

```go
	Description string     // for TaskCreate calls: the task description, rendered as markdown
	TaskNumber  string     // for TaskCreate calls: the created task's number from the result, e.g. "12"
```

In `internal/transcript/parse.go`, extend the `tool_result` case in `buildTurn`:

```go
		case "tool_result":
			if tc := toolIndex[b.ToolUseID]; tc != nil {
				tc.Result = &model.ToolResult{Content: toolResultText(b.Content), IsError: b.IsError}
				if tc.IsTaskCreate() && !tc.Result.IsError {
					tc.TaskNumber = taskNumber(tc.Result.Content)
				}
			}
```

Below `taskDescription`, add:

```go
// taskNumber extracts N from a TaskCreate result like "Task #12 created
// successfully: …". Returns "" when the content does not start that way.
func taskNumber(content string) string {
	rest, ok := strings.CutPrefix(content, "Task #")
	if !ok {
		return ""
	}
	i := 0
	for i < len(rest) && rest[i] >= '0' && rest[i] <= '9' {
		i++
	}
	return rest[:i]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/transcript/ -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/   # must print nothing
git add internal/model/model.go internal/transcript/parse.go internal/transcript/parse_test.go
git commit -m "feat(parse): extract the created task number from TaskCreate results

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 3: Render the cards — title prefix, markdown body, result suppression

**Files:**
- Modify: `internal/render/render.go` (`renderTool` ~line 338)
- Modify: `internal/render/assets/styles.css` (after the `.agent-result-body` rules ~line 62)
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `ToolCall.TaskNumber`, `ToolCall.Description`, `IsTaskCreate()`, `IsTaskUpdate()` from Tasks 1–2; existing `Markdown()`, `renderTurnBody`, `newAgentLinks`.
- Produces: final HTML behavior; unexported helpers `headerSummary(tc *model.ToolCall) string` and `resultRedundant(tc *model.ToolCall) bool`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestTask(Create|Update)' -v`
Expected: FAIL — header lacks `#12 · `, no `task-desc` block, results not suppressed. (Compiles fine; these are behavior failures.)

- [ ] **Step 3: Implement the render changes**

In `internal/render/render.go`, `renderTool`, replace the summary span:

```go
	b.WriteString(`<span class="tool-name">` + html.EscapeString(tc.Name) + `</span>`)
	if s := headerSummary(tc); s != "" {
		b.WriteString(`<span class="tool-summary">` + html.EscapeString(StripANSI(s)) + `</span>`)
	}
```

In the body `switch`, insert a case directly before `case tc.InputJSON != "":`:

```go
	case tc.IsTaskCreate() && tc.Description != "":
		// The task description is markdown; render it readably instead of
		// dumping the raw JSON input.
		b.WriteString(`<div class="task-desc">` + string(Markdown(tc.Description)) + `</div>`)
```

Change the result guard from

```go
	if tc.Result != nil && tc.Result.Content != "" {
```

to

```go
	if tc.Result != nil && tc.Result.Content != "" && !resultRedundant(tc) {
```

Add the two helpers after `renderTool` (before `resultContent`):

```go
// headerSummary is the text shown next to the tool name in the collapsed card
// header: the parse-time Summary, prefixed with the created task's number for
// TaskCreate calls whose result has been seen ("#12 · <subject>"). Summary
// itself stays the plain subject so search text is unaffected.
func headerSummary(tc *model.ToolCall) string {
	switch {
	case tc.TaskNumber == "":
		return tc.Summary
	case tc.Summary == "":
		return "#" + tc.TaskNumber
	default:
		return "#" + tc.TaskNumber + " · " + tc.Summary
	}
}

// resultRedundant reports whether a successful result would only repeat what
// the card title already shows: "Task #N created successfully: …" once the
// number is in a TaskCreate title, "Updated task #N status" once id · status
// is in a TaskUpdate title. Errors are never redundant.
func resultRedundant(tc *model.ToolCall) bool {
	if tc.Result == nil || tc.Result.IsError {
		return false
	}
	return (tc.IsTaskCreate() && tc.TaskNumber != "") || (tc.IsTaskUpdate() && tc.Summary != "")
}
```

In `internal/render/assets/styles.css`, after the `.agent-result-body::before` rule (line 62), add:

```css
.task-desc { border-left: 3px solid var(--accent-soft); padding: 0 .8rem; margin: .2rem 0 .8rem; font-size: .9rem; }
.task-desc::before { content: "description"; display: block; color: var(--muted); font-size: .7rem; text-transform: uppercase; letter-spacing: .05em; margin-bottom: .2rem; }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./... `
Expected: all packages PASS (render e2e tests included — none assert on Task* cards today, but the full run guards regressions).

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/   # must print nothing
git add internal/render/render.go internal/render/assets/styles.css internal/render/render_test.go
git commit -m "feat(render): readable TaskCreate/TaskUpdate cards

TaskCreate titles read '#12 · <subject>' with the description rendered
as markdown; TaskUpdate titles read '#3 · completed'. The result line is
hidden when it only repeats the title; errors still render.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

## Verification (after all tasks)

- `go test ./...` — full suite green.
- `gofmt -l internal/` — no output.
- Manual smoke: `go run ./cmd/ccwhid <a session with TaskCreate calls> -o /tmp/ccwhid-check` (any transcript under `~/.claude/projects/-Users-markus-IdeaProjects-cc-what-have-i-done/` from 2026-07-12 contains TaskCreate/TaskUpdate calls) and eyeball a `TaskCreate` card: title `#1 · <subject>`, markdown body, no duplicate result line. Check `cmd/ccwhid` flags with `--help` first if the invocation differs.
