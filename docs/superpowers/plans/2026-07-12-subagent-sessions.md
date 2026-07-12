# Subagent Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Agent results render as agent-attributed cards (never "You"), linked subagent session transcripts become separate HTML pages, and their usage rolls up into the main usage summary.

**Architecture:** The transcript parser learns to recognize `<task-notification>` user records (new turn kind `agent-result`) and to load `<sessionId>/subagents/agent-*.jsonl` files as `model.AgentSession` values attached to the root `model.Session`. The renderer emits one extra HTML page per agent session under `outDir/subagents/` (same template, `{{ .Base }}`-prefixed asset paths) and links to them from tool cards and result cards. `usage.Compute` additionally walks `s.Agents` and tracks a subagent subtotal.

**Tech Stack:** Go 1.26, html/template, existing packages `internal/{model,transcript,render,usage,redact,tui}`.

**Spec:** `docs/superpowers/specs/2026-07-12-subagent-sessions-design.md`

## Global Constraints

- The task-notification fix (Task 1, Task 4 card rendering) is **unconditional** — independent of `--include-subagents` and `--usage`.
- Linked agent-session loading is gated on `--include-subagents` (existing flag, default `true`).
- Agent page path is exactly `subagents/agent-<ID>.html` relative to the output dir.
- A malformed notification, missing `subagents/` dir, or missing/unreadable `meta.json` must never fail the run (fallbacks per task).
- Flat SDK-spawned session files stay out of scope; do not touch discovery classification.
- Run `gofmt -l .` (must print nothing) before every commit.
- Every commit message ends with a `Co-Authored-By:` trailer identifying the authoring model.

---

### Task 1: Parse `<task-notification>` records into agent-result turns

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/transcript/parse.go`
- Modify: `internal/redact/redact.go` (redact the new summary field)
- Test: `internal/transcript/notification_test.go` (new)

**Interfaces:**
- Consumes: existing `buildTurn`, `apiBlock`, `model.Turn`.
- Produces: `model.TurnAgentResult TurnKind = "agent-result"`; `model.Turn` fields `AgentID, AgentStatus, AgentSummary string`; unexported `parseTaskNotification(s string) (taskNotification, bool)` with `taskNotification{TaskID, ToolUseID, Status, Summary, Result string}`. Task 4 renders turns of this kind; Task 5 links via `Turn.AgentID`.

- [ ] **Step 1: Write the failing test**

Create `internal/transcript/notification_test.go`:

```go
package transcript

import (
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

const notifBody = `<task-notification>
<task-id>acb6584f99f2f81fd</task-id>
<tool-use-id>toolu_0112a19E</tool-use-id>
<output-file>/tmp/tasks/acb6584f99f2f81fd.output</output-file>
<status>completed</status>
<summary>Agent "Implement Task 12: Profiles view" finished</summary>
<note>may notify more than once</note>
<result>Both items fixed.

## Status: Complete</result>
</task-notification>`

func notifLine(t *testing.T, body string) string {
	t.Helper()
	b, err := jsonMarshalString(body)
	if err != nil {
		t.Fatal(err)
	}
	return `{"type":"user","timestamp":"2026-07-05T18:26:08.000Z","message":{"role":"user","content":` + b + `}}`
}

func TestParseTaskNotificationTurn(t *testing.T) {
	s, err := Parse(strings.NewReader(notifLine(t, notifBody)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(s.Turns))
	}
	turn := s.Turns[0]
	if turn.Kind != model.TurnAgentResult {
		t.Fatalf("kind = %q, want %q", turn.Kind, model.TurnAgentResult)
	}
	if turn.AgentID != "acb6584f99f2f81fd" {
		t.Errorf("AgentID = %q", turn.AgentID)
	}
	if turn.AgentStatus != "completed" {
		t.Errorf("AgentStatus = %q", turn.AgentStatus)
	}
	if want := `Agent "Implement Task 12: Profiles view" finished`; turn.AgentSummary != want {
		t.Errorf("AgentSummary = %q", turn.AgentSummary)
	}
	if len(turn.Blocks) != 1 || !strings.Contains(turn.Blocks[0].Text, "## Status: Complete") {
		t.Errorf("body block wrong: %+v", turn.Blocks)
	}
	if strings.Contains(turn.Blocks[0].Text, "<task-id>") {
		t.Error("body must be the <result> payload only, not the raw envelope")
	}
}

func TestParseTaskNotificationEmptyResultFallsBackToSummary(t *testing.T) {
	body := "<task-notification>\n<task-id>a1</task-id>\n<status>completed</status>\n<summary>Agent \"x\" finished</summary>\n<result></result>\n</task-notification>"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Blocks[0].Text != `Agent "x" finished` {
		t.Fatalf("want summary as body, got %+v", s.Turns)
	}
}

func TestParseMalformedNotificationStaysUserTurn(t *testing.T) {
	// No <task-id> -> must stay a plain user turn, content untouched.
	body := "<task-notification>\nbroken payload\n</task-notification>"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Kind != model.TurnUser {
		t.Fatalf("want plain user turn, got %+v", s.Turns)
	}
}

func TestUserTextMentioningNotificationMidStringStaysUserTurn(t *testing.T) {
	body := "please explain what a <task-notification> record is"
	s, err := Parse(strings.NewReader(notifLine(t, body)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Kind != model.TurnUser {
		t.Fatalf("want plain user turn, got %+v", s.Turns)
	}
}

func TestRepeatNotificationsEachProduceATurn(t *testing.T) {
	line := notifLine(t, notifBody)
	s, err := Parse(strings.NewReader(line+"\n"+line), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 {
		t.Fatalf("got %d turns, want 2 (no dedup)", len(s.Turns))
	}
}
```

Add the small helper at the bottom of the same test file:

```go
// jsonMarshalString wraps a string as a JSON string literal.
func jsonMarshalString(s string) (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}
```

with `"encoding/json"` added to the test file's imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'Notification|NotificationTurn|StaysUserTurn|EachProduceATurn' -v`
Expected: FAIL — `model.TurnAgentResult` undefined (compile error).

- [ ] **Step 3: Add model fields**

In `internal/model/model.go`, extend the turn-kind consts:

```go
const (
	TurnUser        TurnKind = "user"
	TurnAssistant   TurnKind = "assistant"
	TurnAgentResult TurnKind = "agent-result" // background-agent completion fed back into the session
)
```

Extend `Turn` (after the `Usage` field):

```go
	// Agent-result fields, set only when Kind == TurnAgentResult.
	AgentID      string // task/agent id from the notification
	AgentStatus  string // e.g. "completed"
	AgentSummary string // e.g. `Agent "Implement Task 12" finished`
```

- [ ] **Step 4: Implement notification parsing**

In `internal/transcript/parse.go`, add (near `buildTurn`):

```go
// taskNotification is the parsed payload of a <task-notification> user record —
// the message a background agent's completion injects into the parent session.
type taskNotification struct {
	TaskID    string
	ToolUseID string
	Status    string
	Summary   string
	Result    string
}

// parseTaskNotification extracts fields from a <task-notification> payload with
// tolerant string scanning (the payload is pseudo-XML with unescaped markdown
// inside <result>). ok is false unless both task-id and summary are present.
func parseTaskNotification(s string) (taskNotification, bool) {
	t := trimLeftSpace(s)
	if !hasPrefix(t, "<task-notification>") {
		return taskNotification{}, false
	}
	n := taskNotification{
		TaskID:    tagContent(t, "task-id"),
		ToolUseID: tagContent(t, "tool-use-id"),
		Status:    tagContent(t, "status"),
		Summary:   tagContent(t, "summary"),
		Result:    tagContent(t, "result"),
	}
	if n.TaskID == "" || n.Summary == "" {
		return taskNotification{}, false
	}
	return n, true
}

// tagContent returns the text between the first <name> and the last </name>,
// trimmed. The last closing tag guards <result> bodies that themselves contain
// XML-looking text.
func tagContent(s, name string) string {
	open, close := "<"+name+">", "</"+name+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	j := strings.LastIndex(rest, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

func trimLeftSpace(s string) string { return strings.TrimLeft(s, " \t\r\n") }

func hasPrefix(s, p string) bool { return strings.HasPrefix(s, p) }
```

Add `"strings"` to `parse.go`'s imports. (If you prefer, call `strings.HasPrefix`/`strings.TrimLeft` directly and drop the two one-line wrappers — either is fine; do not add other helpers.)

In `buildTurn`, immediately after the `turn.Kind` assignment block, add:

```go
	// A background agent's completion arrives as a user record whose content is
	// a single <task-notification> text payload. Surface it as an agent-result
	// turn so the report attributes it to the agent, not to the user.
	if turn.Kind == model.TurnUser && len(blocks) == 1 && blocks[0].Type == "text" {
		if n, ok := parseTaskNotification(blocks[0].Text); ok {
			turn.Kind = model.TurnAgentResult
			turn.AgentID = n.TaskID
			turn.AgentStatus = n.Status
			turn.AgentSummary = n.Summary
			body := n.Result
			if body == "" {
				body = n.Summary
			}
			turn.Blocks = []model.Block{{Type: model.BlockText, Text: body}}
			return turn
		}
	}
```

- [ ] **Step 5: Redact the new field**

In `internal/redact/redact.go`, function `redactTurn`, add alongside the existing per-turn string fields:

```go
	t.AgentSummary = r.String(t.AgentSummary)
```

(Open the file first and place it with the other `r.String` rewrites at the top of `redactTurn`; the result body is a normal text block and is already covered.)

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/transcript/ ./internal/redact/ -v`
Expected: all PASS, including the five new tests.

- [ ] **Step 7: Run the full suite and commit**

Run: `gofmt -l . && go test ./...`
Expected: gofmt prints nothing; all packages pass.

```bash
git add internal/model/model.go internal/transcript/parse.go internal/transcript/notification_test.go internal/redact/redact.go
git commit -m "feat: parse task-notification records as agent-result turns"
```

---

### Task 2: Load linked agent-session files

**Files:**
- Modify: `internal/model/model.go`
- Modify: `internal/transcript/parse.go` (Options field, sidechain override)
- Create: `internal/transcript/agents.go`
- Modify: `internal/redact/redact.go` (walk `s.Agents`)
- Test: `internal/transcript/agents_test.go` (new)

**Interfaces:**
- Consumes: `ParseFile(path string, opts Options) (model.Session, error)` (existing), `model.Session`.
- Produces:
  - `model.AgentSession{ID, Description, AgentType, ToolUseID string; SpawnDepth int; Session Session}` and `model.Session.Agents []AgentSession`.
  - `transcript.Options.AgentFile bool` — when set, `isSidechain` on records is ignored (an agent's own file IS its main chain).
  - `func LoadAgentSessions(transcriptPath string, opts Options) ([]model.AgentSession, error)` in package `transcript` — reads `<transcriptPath minus .jsonl>/subagents/agent-*.jsonl`, sorted by file name; missing dir → `(nil, nil)`; unparsable file → skip with stderr warning; missing/unreadable meta.json → Description falls back to ID.

- [ ] **Step 1: Write the failing test**

Create `internal/transcript/agents_test.go`:

```go
package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// agentRecord is a minimal agent-file line: sidechain-flagged user text.
const agentRecord = `{"type":"user","isSidechain":true,"timestamp":"2026-07-05T18:11:19.567Z","message":{"role":"user","content":"do the thing"}}`

func writeAgentFixture(t *testing.T) (rootPath string) {
	t.Helper()
	dir := t.TempDir()
	rootPath = filepath.Join(dir, "root-session.jsonl")
	if err := os.WriteFile(rootPath, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "root-session", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"agent-bbb222.jsonl":     agentRecord + "\n",
		"agent-aaa111.jsonl":     agentRecord + "\n",
		"agent-aaa111.meta.json": `{"agentType":"general-purpose","description":"Implement Task 12","toolUseId":"toolu_01x","spawnDepth":1}`,
		"agent-broken.jsonl":     "", // empty file parses to an empty session, still listed
		"notes.txt":              "ignore me",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sub, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rootPath
}

func TestLoadAgentSessions(t *testing.T) {
	root := writeAgentFixture(t)
	agents, err := LoadAgentSessions(root, Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(agents))
	}
	// Sorted by file name: aaa111, bbb222, broken.
	a := agents[0]
	if a.ID != "aaa111" || a.Description != "Implement Task 12" || a.AgentType != "general-purpose" || a.ToolUseID != "toolu_01x" || a.SpawnDepth != 1 {
		t.Errorf("meta not applied: %+v", a)
	}
	if len(a.Session.Turns) != 1 || a.Session.Turns[0].Kind != model.TurnUser {
		t.Errorf("agent transcript not parsed as main chain: %+v", a.Session.Turns)
	}
	// No meta.json -> Description falls back to ID.
	if agents[1].ID != "bbb222" || agents[1].Description != "bbb222" {
		t.Errorf("fallback description wrong: %+v", agents[1])
	}
}

func TestLoadAgentSessionsMissingDir(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "lonely.jsonl")
	if err := os.WriteFile(root, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents, err := LoadAgentSessions(root, Options{})
	if err != nil || agents != nil {
		t.Fatalf("want (nil, nil), got (%v, %v)", agents, err)
	}
}

func TestAgentFileOptionParsesSidechainAsMainChain(t *testing.T) {
	s, err := Parse(strings.NewReader(agentRecord), Options{AgentFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("AgentFile must parse sidechain records as main chain; got %d turns", len(s.Turns))
	}
	// Without the option the sidechain record is dropped (no owner).
	s2, err := Parse(strings.NewReader(agentRecord), Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Turns) != 0 {
		t.Fatalf("without AgentFile the record must not become a main turn; got %d", len(s2.Turns))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'AgentSessions|AgentFile' -v`
Expected: FAIL — `LoadAgentSessions` and `Options.AgentFile` undefined (compile error).

- [ ] **Step 3: Add the model type**

In `internal/model/model.go`, add to `Session`:

```go
	Agents       []AgentSession // linked subagent sessions (subagents/ dir), when loaded
```

and after the `Subagent` type:

```go
// AgentSession is a subagent session file linked to a root session
// (<projectDir>/<sessionId>/subagents/agent-<ID>.jsonl).
type AgentSession struct {
	ID          string  // agentId, from the file name agent-<ID>.jsonl
	Description string  // from meta.json; falls back to ID
	AgentType   string  // from meta.json; may be empty
	ToolUseID   string  // id of the spawning tool call; may be empty
	SpawnDepth  int     // from meta.json; informational only
	Session     Session // the parsed agent transcript
}
```

- [ ] **Step 4: Add the AgentFile option**

In `internal/transcript/parse.go`:

```go
// Options controls how a transcript is folded into a Session.
type Options struct {
	IncludeSubagents bool
	// AgentFile marks the input as an agent's own transcript file: its records
	// carry isSidechain=true but form the file's main chain, so the flag is
	// ignored.
	AgentFile bool
}
```

In `Parse`, at the top of the per-record handling (right after the `rec.IsMeta` check), add:

```go
		if opts.AgentFile {
			rec.IsSidechain = false
		}
```

- [ ] **Step 5: Implement the loader**

Create `internal/transcript/agents.go`:

```go
package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// agentMeta mirrors agent-<ID>.meta.json next to each agent transcript.
type agentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	ToolUseID   string `json:"toolUseId"`
	SpawnDepth  int    `json:"spawnDepth"`
}

// LoadAgentSessions parses the linked subagent transcripts of a root session:
// <transcriptPath minus .jsonl>/subagents/agent-*.jsonl, sorted by file name.
// A missing subagents dir yields (nil, nil); an unparsable agent file is
// skipped with a warning on stderr. Nested agents (spawnDepth > 1) live in the
// same flat directory and are returned alongside depth-1 agents.
func LoadAgentSessions(transcriptPath string, opts Options) ([]model.AgentSession, error) {
	dir := filepath.Join(strings.TrimSuffix(transcriptPath, ".jsonl"), "subagents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	opts.AgentFile = true
	var out []model.AgentSession
	for _, e := range entries { // ReadDir sorts by name
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl")
		sess, err := ParseFile(filepath.Join(dir, name), opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping agent transcript %s: %v\n", name, err)
			continue
		}
		a := model.AgentSession{ID: id, Description: id, Session: sess}
		if b, err := os.ReadFile(filepath.Join(dir, "agent-"+id+".meta.json")); err == nil {
			var m agentMeta
			if json.Unmarshal(b, &m) == nil {
				if m.Description != "" {
					a.Description = m.Description
				}
				a.AgentType = m.AgentType
				a.ToolUseID = m.ToolUseID
				a.SpawnDepth = m.SpawnDepth
			}
		}
		out = append(out, a)
	}
	return out, nil
}
```

- [ ] **Step 6: Redact agent sessions**

In `internal/redact/redact.go`, at the end of `func Session(s *model.Session, homeDir string)`, add:

```go
	for i := range s.Agents {
		s.Agents[i].Description = r.String(s.Agents[i].Description)
		Session(&s.Agents[i].Session, homeDir)
	}
```

Note: `Session` creates a new Redactor per call; that is fine (it is cheap and keeps the recursion simple).

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/transcript/ ./internal/redact/ -v`
Expected: PASS.

- [ ] **Step 8: Run the full suite and commit**

Run: `gofmt -l . && go test ./...`
Expected: gofmt prints nothing; all pass.

```bash
git add internal/model/model.go internal/transcript/parse.go internal/transcript/agents.go internal/transcript/agents_test.go internal/redact/redact.go
git commit -m "feat: load linked subagent session transcripts"
```

---

### Task 3: Usage rollup for agent sessions

**Files:**
- Modify: `internal/usage/usage.go`
- Test: `internal/usage/usage_test.go`

**Interfaces:**
- Consumes: `model.Session.Agents` (Task 2), existing `Compute`, `walk`, `Lookup`, `stripDateSuffix`, `cost`.
- Produces: `usage.Report` gains

  ```go
  Subagents     TokenCounts // tokens from linked agent sessions
  SubagentsCost *float64    // nil when nothing in them was priced
  AgentSessions int         // number of linked agent sessions
  ```

  Task 4's usage view reads these three fields.

- [ ] **Step 1: Write the failing test**

Append to `internal/usage/usage_test.go`:

```go
func TestComputeRollsUpLinkedAgentSessions(t *testing.T) {
	agent := model.AgentSession{
		ID: "a1",
		Session: model.Session{Turns: []model.Turn{
			{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: u(1_000_000, 0, 0, 0, 0)},
		}},
	}
	s := model.Session{
		Turns: []model.Turn{
			{Kind: model.TurnAssistant, Model: "claude-opus-4-8", Usage: u(1_000_000, 0, 0, 0, 0)},
		},
		Agents: []model.AgentSession{agent},
	}
	r := Compute(s)
	if r.Total.Input != 2_000_000 {
		t.Errorf("Total.Input = %d, want 2000000 (main + agent)", r.Total.Input)
	}
	if r.Subagents.Input != 1_000_000 {
		t.Errorf("Subagents.Input = %d, want 1000000", r.Subagents.Input)
	}
	if r.AgentSessions != 1 {
		t.Errorf("AgentSessions = %d, want 1", r.AgentSessions)
	}
	// opus-4-8 input $5/MTok -> subagent cost $5.00.
	if r.SubagentsCost == nil || *r.SubagentsCost < 4.99 || *r.SubagentsCost > 5.01 {
		t.Errorf("SubagentsCost = %v, want ~5.00", r.SubagentsCost)
	}
	// Per-model table merges both (one opus row with 2M input).
	if len(r.ByModel) != 1 || r.ByModel[0].Tokens.Input != 2_000_000 {
		t.Errorf("ByModel = %+v, want single merged opus row with 2M input", r.ByModel)
	}
	// Agent turns must not create per-turn cost entries.
	if len(r.PerTurnCost) != 1 {
		t.Errorf("PerTurnCost entries = %d, want 1 (main turn only)", len(r.PerTurnCost))
	}
}

func TestComputeUnpricedAgentSessionHasNilSubagentsCost(t *testing.T) {
	s := model.Session{
		Agents: []model.AgentSession{{
			ID: "a1",
			Session: model.Session{Turns: []model.Turn{
				{Kind: model.TurnAssistant, Model: "mystery-model", Usage: u(100, 0, 0, 0, 0)},
			}},
		}},
	}
	r := Compute(s)
	if r.SubagentsCost != nil {
		t.Errorf("SubagentsCost = %v, want nil (unpriced)", *r.SubagentsCost)
	}
	if r.Subagents.Input != 100 || r.AgentSessions != 1 {
		t.Errorf("subtotal wrong: %+v sessions=%d", r.Subagents, r.AgentSessions)
	}
	if !r.HasUnknownModel {
		t.Error("HasUnknownModel should be true")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/usage/ -run RollsUp -v`
Expected: FAIL — `Report` has no `Subagents`/`SubagentsCost`/`AgentSessions` fields (compile error).

- [ ] **Step 3: Implement the rollup**

In `internal/usage/usage.go`, extend `Report`:

```go
	Subagents     TokenCounts // tokens from linked agent sessions
	SubagentsCost *float64    // nil when nothing in them was priced
	AgentSessions int         // number of linked agent sessions
```

In `Compute`, make the accumulator subagent-aware. Add just before `accumulate` is defined:

```go
	subByModel := map[string]*TokenCounts{}
	inAgent := false
```

Inside `accumulate`, after `tc.add(*t.Usage)`, add:

```go
		if inAgent {
			r.Subagents.add(*t.Usage)
			stc, ok := subByModel[key]
			if !ok {
				stc = &TokenCounts{}
				subByModel[key] = stc
			}
			stc.add(*t.Usage)
		}
```

After the existing "Totals + per-model" loop over `s.Turns` (and before the per-turn cost loop), add:

```go
	// Linked agent sessions merge into the same totals and per-model rows,
	// tracked separately so the report can show the subagent share.
	r.AgentSessions = len(s.Agents)
	inAgent = true
	for _, a := range s.Agents {
		for _, t := range a.Session.Turns {
			walk(t, accumulate)
		}
	}
	var subTotal float64
	subPriced := false
	for m, tc := range subByModel {
		if p, ok := Lookup(m); ok {
			subTotal += cost(*tc, p)
			subPriced = true
		}
	}
	if subPriced {
		r.SubagentsCost = &subTotal
	}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/usage/ -v`
Expected: all PASS (existing tests unaffected: with no `Agents`, `Subagents` stays zero and `SubagentsCost` nil).

- [ ] **Step 5: Run the full suite and commit**

Run: `gofmt -l . && go test ./...`
Expected: gofmt prints nothing; all pass.

```bash
git add internal/usage/usage.go internal/usage/usage_test.go
git commit -m "feat: roll linked agent-session usage into the usage report"
```

---

### Task 4: Render agent-result cards and the subagent usage line

**Files:**
- Modify: `internal/render/render.go`
- Modify: `internal/render/assets/report.html.tmpl`
- Modify: `internal/render/assets/styles.css`
- Test: `internal/render/render_test.go` (append)

**Interfaces:**
- Consumes: `model.TurnAgentResult`, `Turn.AgentStatus/AgentSummary` (Task 1); `usage.Report.Subagents/SubagentsCost/AgentSessions` (Task 3).
- Produces: `turnView` gains `Status string`; `usageView` gains `SubLine string`; unexported `agentRoleLabel(summary string) string`. Task 5 extends the same files further.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go` (check the file's existing imports; it already imports `strings`, `testing`, and the `model` package — add `usage` if absent):

```go
func TestAgentResultTurnRendersAsAgentCard(t *testing.T) {
	s := model.Session{Turns: []model.Turn{{
		Kind:         model.TurnAgentResult,
		AgentStatus:  "completed",
		AgentSummary: `Agent "Implement Task 12: Profiles view" finished`,
		Blocks:       []model.Block{{Type: model.BlockText, Text: "All done."}},
	}}}
	d := buildViewModel(s, "t", Options{})
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'AgentResult|AgentRoleLabel|SubagentLine|NoSubagentLine' -v`
Expected: FAIL — `turnView` has no `Status`, `usageView` has no `SubLine`, `agentRoleLabel` undefined (compile errors).

- [ ] **Step 3: Implement the card and usage-line rendering**

In `internal/render/render.go`:

1. Extend `turnView`:

```go
type turnView struct {
	Index      int
	Kind       string
	RoleLabel  string
	Status     string // agent-result status chip, e.g. "completed"
	SearchText string
	Body       template.HTML
	Badge      string // per-turn usage badge, e.g. "12k tok · ~$0.18"
}
```

2. Extend `usageView` with `SubLine string` (after `Total`):

```go
	SubLine  string // "of which subagents: …", empty when no linked agents
```

3. Replace `roleLabel` and add `agentRoleLabel`:

```go
func roleLabel(t model.Turn) string {
	switch t.Kind {
	case model.TurnUser:
		return "You"
	case model.TurnAgentResult:
		return agentRoleLabel(t.AgentSummary)
	default:
		return "Claude"
	}
}

// agentRoleLabel derives `Agent · <name>` from a notification summary like
// `Agent "Implement Task 12" finished`; plain "Agent" when no quoted name.
func agentRoleLabel(summary string) string {
	if i := strings.Index(summary, `"`); i >= 0 {
		if j := strings.Index(summary[i+1:], `"`); j > 0 {
			return "Agent · " + summary[i+1:i+1+j]
		}
	}
	return "Agent"
}
```

Update the two existing `roleLabel(...)` call sites to pass the turn:
- in `buildViewModel`: `RoleLabel: roleLabel(t)` and add `Status: t.AgentStatus`
- in `renderTool` (subagent inline rendering): `roleLabel(st)`

4. In `buildUsageView`, after `v.Total = modelRow(...)`, add:

```go
	if r.AgentSessions > 0 {
		sub := "of which subagents: " + formatTokens(r.Subagents.InOut()) + " in+out"
		if r.SubagentsCost != nil {
			sub += " · ~" + formatCost(*r.SubagentsCost)
		}
		v.SubLine = fmt.Sprintf("%s (%d sessions)", sub, r.AgentSessions)
	}
```

and change the footnote block to:

```go
	foot := "Estimated — Anthropic list prices as of " + r.PricesAsOf + "."
	if r.AgentSessions > 0 {
		foot += fmt.Sprintf(" Includes %d linked subagent session(s); server-tool fees are excluded.", r.AgentSessions)
	} else {
		foot += " Covers this transcript only; sub-agent sessions stored as separate files, and server-tool fees, are excluded."
	}
	if hasUnpriced {
		foot += " Estimated cost excludes unpriced models (shown as n/a); their tokens are still counted."
	}
	v.Footnote = foot
```

Add `"fmt"` to render.go's imports.

5. In `assets/report.html.tmpl`:

- Turn header line becomes:

```html
      <div class="turn-role">{{ .RoleLabel }}{{ if .Status }}<span class="agent-status">{{ .Status }}</span>{{ end }}{{ if .Badge }}<span class="usage-badge">{{ .Badge }}</span>{{ end }}</div>
```

- Below the usage table (before the footnote paragraph):

```html
        {{ if .SubLine }}<p class="usage-sub">{{ .SubLine }}</p>{{ end }}
```

6. In `assets/styles.css`, append:

```css
.turn-agent-result { background: var(--surface); border-left: 3px solid var(--accent); }
.turn-agent-result .turn-role { color: var(--accent); }
.agent-status { margin-left: 0.5rem; padding: 0 0.4rem; border: 1px solid var(--border); border-radius: 999px; color: var(--muted); font-size: 0.7rem; text-transform: none; letter-spacing: normal; }
.usage-sub { color: var(--muted); font-size: 0.85rem; margin-top: 0.5rem; }
```

(The card class `turn-agent-result` comes for free: the template already emits `turn-{{ .Kind }}` and the kind string is `agent-result`.)

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/render/ -v`
Expected: all PASS (existing render tests keep passing — `roleLabel` call sites updated).

- [ ] **Step 5: Run the full suite and commit**

Run: `gofmt -l . && go test ./...`
Expected: gofmt prints nothing; all pass.

```bash
git add internal/render/render.go internal/render/render_test.go internal/render/assets/report.html.tmpl internal/render/assets/styles.css
git commit -m "feat: agent-attributed result cards and subagent usage line"
```

---

### Task 5: Subagent pages, links, and wiring

**Files:**
- Modify: `internal/render/render.go`
- Modify: `internal/render/assets/report.html.tmpl`
- Modify: `internal/render/assets/styles.css`
- Modify: `cmd/ccwhid/run.go`
- Modify: `cmd/ccwhid/main.go` (flag help text)
- Modify: `internal/tui/tui.go` (option label)
- Modify: `README.md`
- Test: `internal/render/render_e2e_test.go` (append)

**Interfaces:**
- Consumes: `model.Session.Agents`, `model.AgentSession` (Task 2), `Turn.AgentID` (Task 1), `transcript.LoadAgentSessions` (Task 2).
- Produces: `render.Site` writes `outDir/subagents/agent-<ID>.html` per agent session; main `index.html` links to them from Task/Agent tool cards (`ToolUseID` match) and agent-result cards (`AgentID` match); agent pages link back via `../index.html` and to sibling pages.

- [ ] **Step 1: Write the failing e2e test**

Append to `internal/render/render_e2e_test.go` (it already renders a session to a temp dir; follow its existing helper style — read the file first):

```go
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
	if err := Site(s, dir, Options{}); err != nil {
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
	if err := Site(model.Session{Title: "x"}, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "subagents")); !os.IsNotExist(err) {
		t.Errorf("subagents dir must not exist for agent-less sessions (err=%v)", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'SubagentPages|NoSubagentsDir' -v`
Expected: FAIL — no agent page is written / no links found.

- [ ] **Step 3: Implement multi-page rendering and links**

In `internal/render/render.go`:

1. Add a page-context type and link resolver:

```go
// pageInfo describes where the page being rendered lives relative to outDir.
type pageInfo struct {
	Base       string // asset prefix: "" on index.html, "../" on agent pages
	PagePrefix string // href prefix to agent pages: "subagents/" on index, "" on agent pages
	BackHref   string // back-link to the main report; empty on index
	Subtitle   string // agent pages: "<agentType> · agent-<ID>"
}

// agentLinks resolves tool-use ids and agent ids to agent-page hrefs.
type agentLinks struct {
	prefix    string
	byToolUse map[string]string // tool_use id -> agent id
	byAgentID map[string]bool
}

func newAgentLinks(agents []model.AgentSession, prefix string) *agentLinks {
	l := &agentLinks{prefix: prefix, byToolUse: map[string]string{}, byAgentID: map[string]bool{}}
	for _, a := range agents {
		if a.ToolUseID != "" {
			l.byToolUse[a.ToolUseID] = a.ID
		}
		l.byAgentID[a.ID] = true
	}
	return l
}

func (l *agentLinks) forToolUse(id string) string {
	if aid, ok := l.byToolUse[id]; ok {
		return l.prefix + "agent-" + aid + ".html"
	}
	return ""
}

func (l *agentLinks) forAgent(id string) string {
	if l.byAgentID[id] {
		return l.prefix + "agent-" + id + ".html"
	}
	return ""
}
```

2. Thread the links through body rendering. Change signatures:

```go
func renderTurnBody(t model.Turn, links *agentLinks) template.HTML
func renderTool(tc *model.ToolCall, links *agentLinks) string
```

- `renderTurnBody` passes `links` to `renderTool`; its recursive call for inline sidechain turns passes `links` unchanged.
- In `renderTool`, after the `tool-summary` span inside the summary element, add:

```go
	if href := links.forToolUse(tc.ID); href != "" {
		b.WriteString(`<a class="agent-link" href="` + html.EscapeString(href) + `">transcript ↗</a>`)
	}
```

3. Extend `viewData` and `turnView`:

```go
// viewData gains:
	Base     string
	BackHref string
	Subtitle string
// turnView gains:
	AgentHref string // link to the agent's transcript page, when one exists
```

4. Change `buildViewModel` to accept the page context and links:

```go
func buildViewModel(s model.Session, title string, opts Options, page pageInfo, links *agentLinks) viewData {
	d := viewData{
		Title:     title,
		Session:   s,
		TurnCount: len(s.Turns),
		Base:      page.Base,
		BackHref:  page.BackHref,
		Subtitle:  page.Subtitle,
	}
	...
	tv := turnView{
		...
		Body: renderTurnBody(t, links),
	}
	if t.Kind == model.TurnAgentResult {
		tv.AgentHref = links.forAgent(t.AgentID)
	}
	...
}
```

Existing unit tests that call `buildViewModel(s, "t", Options{})` must be updated to `buildViewModel(s, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""))` — update Task 4's tests accordingly in this task (they are in the same package; keep the assertions unchanged).

5. Restructure `Site` to render index + one page per agent:

```go
// Site renders the session into outDir as index.html + assets/, plus one page
// per linked agent session under subagents/.
func Site(s model.Session, outDir string, opts Options) error {
	title := opts.Title
	if title == "" {
		title = s.DisplayTitle()
	}
	tmplSrc, err := assets.ReadFile("assets/report.html.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("report").Parse(string(tmplSrc))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0o755); err != nil {
		return err
	}
	for _, name := range []string{"styles.css", "app.js"} {
		b, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "assets", name), b, 0o644); err != nil {
			return err
		}
	}

	writePage := func(path string, sess model.Session, pageTitle string, page pageInfo) error {
		links := newAgentLinks(s.Agents, page.PagePrefix)
		data := buildViewModel(sess, pageTitle, opts, page, links)
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return err
		}
		return os.WriteFile(path, buf.Bytes(), 0o644)
	}

	if err := writePage(filepath.Join(outDir, "index.html"), s, title,
		pageInfo{Base: "", PagePrefix: "subagents/"}); err != nil {
		return err
	}
	if len(s.Agents) == 0 {
		return nil
	}
	subDir := filepath.Join(outDir, "subagents")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		return err
	}
	for _, a := range s.Agents {
		sub := "agent-" + a.ID
		if a.AgentType != "" {
			sub = a.AgentType + " · " + sub
		}
		if err := writePage(filepath.Join(subDir, "agent-"+a.ID+".html"), a.Session, a.Description,
			pageInfo{Base: "../", PagePrefix: "", BackHref: "../index.html", Subtitle: sub}); err != nil {
			return err
		}
	}
	return nil
}
```

Note the resolver on agent pages uses the ROOT session's agent set (`s.Agents`) with prefix `""`, so depth-2 agent references inside an agent page resolve to sibling files.

6. In `assets/report.html.tmpl`:

- Asset paths: `href="{{ .Base }}assets/styles.css"` and `src="{{ .Base }}assets/app.js"`.
- In `session-head`, after the `<h1>`:

```html
      {{ if .BackHref }}<a class="back-link" href="{{ .BackHref }}">← main session</a>{{ end }}
```

  and in the `.meta` div add `{{ if .Subtitle }}<span>{{ .Subtitle }}</span>{{ end }}`.
- Turn header: add the transcript link after the status chip:

```html
{{ if .AgentHref }}<a class="agent-link" href="{{ .AgentHref }}">transcript ↗</a>{{ end }}
```

7. In `assets/styles.css`, append:

```css
.agent-link { margin-left: 0.5rem; font-size: 0.75rem; text-transform: none; letter-spacing: normal; }
.back-link { display: inline-block; font-size: 0.85rem; margin-bottom: 0.4rem; }
```

- [ ] **Step 4: Run the render tests**

Run: `go test ./internal/render/ -v`
Expected: all PASS (including Task 4's updated buildViewModel call sites).

- [ ] **Step 5: Wire loading into the CLI**

In `cmd/ccwhid/run.go`, in `generate`, after the `ParseFile` block (before redaction):

```go
	if opts.includeSubagents {
		agents, err := transcript.LoadAgentSessions(si.FilePath, transcript.Options{
			IncludeSubagents: opts.includeSubagents,
		})
		if err != nil {
			return "", err
		}
		sess.Agents = agents
	}
```

In `cmd/ccwhid/main.go`, update the flag help text:

```go
	f.BoolVar(&opts.includeSubagents, "include-subagents", true, "include subagent work: inline Task sidechains and linked agent-session pages")
```

In `internal/tui/tui.go`, change the option label `"Include subagents"` to `"Include subagent work"`.

- [ ] **Step 6: Update the README**

In `README.md`:
- Flag table row for `--include-subagents`: "Include subagent work: inline Task sidechains and linked agent-session transcript pages under `subagents/` (default true)".
- In the report-description paragraph, mention: background-agent results appear as their own "Agent" cards; linked subagent sessions (from `<sessionId>/subagents/`) render as separate pages linked from the Agent tool card and the result card; with `--usage`, their cost is included and shown as an "of which subagents" line.
- Known limitation note: SDK-spawned sessions without parent linkage (e.g. review-hook runs) are not attributed or included.

- [ ] **Step 7: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: gofmt silent, vet clean, all tests pass.

Manual smoke (optional but recommended): `go run ./cmd/ccwhid --latest --usage --force -o /tmp/ccwhid-smoke && ls /tmp/ccwhid-smoke/subagents | head` on a session known to have a `subagents/` dir.

- [ ] **Step 8: Commit**

```bash
git add internal/render/ cmd/ccwhid/ internal/tui/tui.go README.md
git commit -m "feat: subagent transcript pages linked from the report"
```
