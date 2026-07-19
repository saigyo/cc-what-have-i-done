# TaskCreate Card Rendering — Design

**Status:** approved (2026-07-19)

**Goal:** Render `TaskCreate` tool calls as a readable card: the subject (and
the created task's number) in the collapsed card title, and the description as
formatted markdown in the body — instead of the current raw-JSON dump with an
empty header.

## Motivation

A `TaskCreate` call carries a structured payload and a well-known result shape:

```json
"input": {
  "subject": "Task 1: Parse task-notification records into agent-result turns",
  "description": "Plan task 1 of docs/superpowers/plans/2026-07-12-subagent-sessions.md"
}
```

```
tool_result content: "Task #1 created successfully: Task 1: Parse task-notification records…"
```

Today `toolSummary` has no `TaskCreate` case, so the collapsed card header
shows only the bare tool name; the body is the pretty-printed input JSON and a
redundant result `<pre>` block. The description is markdown and deserves the
same treatment agent prompts already get.

## Behavior

- **Card title:** `TaskCreate` + summary `#<n> · <subject>` once the task
  number is known, e.g. `#1 · Parse task-notification records`. When no result
  exists (or the number cannot be parsed), the summary is just the subject.
- **Card body:** the `description` rendered as markdown (same styling approach
  as the agent prompt: a wrapper div + `Markdown(...)`), replacing the raw
  input JSON for this tool.
- **Result block:** hidden when the task number was successfully extracted and
  the result is not an error — it duplicates the title. Error results (and
  results whose text does not match the expected shape) still render as the
  usual `<pre class="tool-result">` block.

## Design

Follows the existing pattern of structured per-tool fields on `model.ToolCall`
(as done for `Task`/`Agent` prompts and `AskUserQuestion` questions).

### Model (`internal/model/model.go`)

- `ToolCall.Description string` — markdown body, set for `TaskCreate` calls.
- `ToolCall.TaskNumber string` — the created task's number (e.g. `"1"`),
  extracted from the result; empty when unknown.
- Helper `func (t *ToolCall) IsTaskCreate() bool { return t.Name == "TaskCreate" }`,
  matching the existing `IsAgent` / `IsAskUserQuestion` helpers.

### Parsing (`internal/transcript/parse.go`)

- `toolSummary`: add case `"TaskCreate"` → `str(in, "subject")`.
- `buildToolCall`: for `TaskCreate`, set `tc.Description = str(in, "description")`.
- Result attachment (the `tool_result` branch in `buildTurn`): after setting
  `tc.Result`, if the call is a `TaskCreate` and the result is not an error,
  extract the number from a leading `Task #<digits>` in the result content and
  store it in `tc.TaskNumber`.

### Rendering (`internal/render/render.go`)

- Title: in `renderTool`, when `tc.TaskNumber != ""`, the summary span shows
  `#<n> · <Summary>`; otherwise `Summary` as today. (Composed at render time;
  `Summary` itself stays the plain subject so search text is unaffected.)
- Body: a new case in the input `switch` — `tc.IsTaskCreate() && tc.Description != ""`
  renders `<div class="task-desc">` + `Markdown(tc.Description)` instead of the
  raw JSON `<pre>`.
- Result: skip the result block when `tc.IsTaskCreate() && tc.TaskNumber != ""`
  and `!tc.Result.IsError`.

### Out of scope

- `TaskUpdate`, `TaskList`, `TaskGet` cards keep their current generic
  rendering.
- No CSS additions unless the markdown body needs spacing; reuse the
  `agent-prompt` styling pattern if a class is needed.

## Testing

- `toolSummary` case for `TaskCreate` (subject; missing subject → empty).
- Parse test: a `TaskCreate` tool_use + tool_result pair yields
  `Description`, `Summary` = subject, and `TaskNumber` = `"1"`; an error
  result or non-matching text leaves `TaskNumber` empty.
- Render test: card HTML contains `#1 · <subject>` in the header, the
  markdown-rendered description, and no `tool-result` block; with an error
  result the `<pre>` block is still present.
