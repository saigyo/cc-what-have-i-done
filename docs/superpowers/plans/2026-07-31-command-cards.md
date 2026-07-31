# Merged Slash-Command Cards Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A slash command renders as one card containing invocation and output, with one clean sidebar prompt entry — instead of two cards and two markup-riddled entries.

**Architecture:** New `BlockCommand` block type with `Command{Invocation, Output}`. Parse-time: tag extractors in a new `internal/transcript/command.go` recognize the `<command-name>` input record (rewritten to a command block) and the `<local-command-stdout>`/`-stderr` output record (absorbed into the preceding open command turn, or degraded to clean text when orphaned). Render-time: a `command` case in `renderTurnBody` and `turnPlainText`, plus two CSS rules.

**Tech Stack:** Go stdlib only (`strings`); plain `testing`; vanilla CSS.

## Global Constraints

- Invocation text: `<command-name>` content plus, when non-empty after trimming, a space and the `<command-args>` content — `/model`, `/code-review ultra 21`. The `<command-message>` echo is dropped.
- Output extraction: `<local-command-stdout>` and/or `<local-command-stderr>` contents, stdout first, joined with a newline when both non-empty, whitespace-trimmed. Extractors require a matching closing tag; without one the turn passes through unchanged.
- Merge rule: an output record is absorbed only when the last appended main-chain turn is a user turn whose single block is a `BlockCommand` with empty `Output`. Otherwise the output record becomes a plain user turn carrying the extracted text (tags never leak).
- ANSI stays in the model; stripped at render (`StripANSI`), consistent with tool results.
- Sidechain (subagent) records untouched. `<bash-input>`/`<bash-stdout>` records out of scope.
- No new go.mod dependencies. gofmt-clean, `go vet ./...` clean, tests pass with `-race`.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

---

### Task 1: Model type, tag extractors, parser merge

**Files:**
- Modify: `internal/model/model.go` (BlockType consts at :15-21, `Block` struct at :72-77)
- Create: `internal/transcript/command.go`
- Modify: `internal/transcript/parse.go` (main-chain append, the `s.Turns = append(s.Turns, *turn)` site at ~:141)
- Test: `internal/transcript/command_test.go` (new), `internal/transcript/parse_test.go`

**Interfaces:**
- Consumes: `model.Block`, `model.TurnUser`, the parse loop's `turn *model.Turn` before append.
- Produces: `model.BlockCommand BlockType = "command"`; `model.Command{Invocation, Output string}`; `Block.Command *Command` field; unexported helpers `tagContent(text, tag string) (string, bool)`, `parseCommand(text string) (string, bool)`, `parseCommandOutput(text string) (string, bool)`, `userText(t *model.Turn) string`, `isOpenCommand(t *model.Turn) bool` (package transcript). Task 2 renders `Block.Command`.

- [ ] **Step 1: Write the failing tests**

Create `internal/transcript/command_test.go`:

```go
package transcript

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name, text, want string
		ok               bool
	}{
		{"name only", "<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args></command-args>", "/model", true},
		{"name and args", "<command-name>/code-review</command-name><command-args>ultra 21</command-args>", "/code-review ultra 21", true},
		{"args whitespace trimmed", "<command-name> /clear </command-name><command-args>  </command-args>", "/clear", true},
		{"no closing tag", "<command-name>/model", "", false},
		{"no command markup", "just a prompt", "", false},
	}
	for _, c := range cases {
		got, ok := parseCommand(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: parseCommand = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestParseCommandOutput(t *testing.T) {
	cases := []struct {
		name, text, want string
		ok               bool
	}{
		{"stdout", "<local-command-stdout>Set model to \x1b[1mFable 5\x1b[22m</local-command-stdout>", "Set model to \x1b[1mFable 5\x1b[22m", true},
		{"stderr only", "<local-command-stderr>boom</local-command-stderr>", "boom", true},
		{"stdout and stderr", "<local-command-stdout>out</local-command-stdout><local-command-stderr>err</local-command-stderr>", "out\nerr", true},
		{"empty stdout", "<local-command-stdout></local-command-stdout>", "", true},
		{"no closing tag", "<local-command-stdout>oops", "", false},
		{"no output markup", "hello", "", false},
	}
	for _, c := range cases {
		got, ok := parseCommandOutput(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: parseCommandOutput = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
```

Append to `internal/transcript/parse_test.go`:

```go
func TestParseSlashCommandMergesOutput(t *testing.T) {
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args></command-args>"},"timestamp":"2026-07-19T18:26:09Z"}`,
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout>Set model to [1mFable 5[22m</local-command-stdout>"},"timestamp":"2026-07-19T18:26:09Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("got %d turns, want 1 (output merged into command turn)", len(s.Turns))
	}
	turn := s.Turns[0]
	if turn.Kind != model.TurnUser || len(turn.Blocks) != 1 {
		t.Fatalf("turn = kind %q with %d blocks, want user with 1", turn.Kind, len(turn.Blocks))
	}
	blk := turn.Blocks[0]
	if blk.Type != model.BlockCommand || blk.Command == nil {
		t.Fatalf("block type = %q (Command %v), want command block", blk.Type, blk.Command)
	}
	if blk.Command.Invocation != "/model" {
		t.Errorf("Invocation = %q, want %q", blk.Command.Invocation, "/model")
	}
	if blk.Command.Output != "Set model to \x1b[1mFable 5\x1b[22m" {
		t.Errorf("Output = %q, want ANSI preserved in model", blk.Command.Output)
	}
}

func TestParseSlashCommandWithoutOutput(t *testing.T) {
	line := `{"type":"user","message":{"role":"user","content":"<command-name>/clear</command-name><command-args></command-args>"},"timestamp":"2026-07-19T18:26:09Z"}`
	s, err := Parse(strings.NewReader(line), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 || s.Turns[0].Blocks[0].Type != model.BlockCommand {
		t.Fatalf("want a single command turn, got %+v", s.Turns)
	}
	if out := s.Turns[0].Blocks[0].Command.Output; out != "" {
		t.Errorf("Output = %q, want empty (no output record)", out)
	}
}

func TestParseOrphanCommandOutputStaysCleanTurn(t *testing.T) {
	// A normal prompt precedes the stdout record: nothing to attach to.
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"a normal prompt"},"timestamp":"2026-07-19T18:26:09Z"}`,
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout>orphaned</local-command-stdout>"},"timestamp":"2026-07-19T18:26:10Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 2 {
		t.Fatalf("got %d turns, want 2 (orphan kept)", len(s.Turns))
	}
	blk := s.Turns[1].Blocks[0]
	if blk.Type != model.BlockText || blk.Text != "orphaned" {
		t.Errorf("orphan block = %q %q, want plain text %q with tags stripped", blk.Type, blk.Text, "orphaned")
	}
}

func TestParseCommandOutputNotAttachedAcrossPrompt(t *testing.T) {
	// command, then a normal prompt, then stdout: the stdout must NOT jump
	// back over the prompt to the command turn.
	lines := strings.Join([]string{
		`{"type":"user","message":{"role":"user","content":"<command-name>/model</command-name><command-args></command-args>"},"timestamp":"2026-07-19T18:26:09Z"}`,
		`{"type":"user","message":{"role":"user","content":"unrelated prompt"},"timestamp":"2026-07-19T18:26:10Z"}`,
		`{"type":"user","message":{"role":"user","content":"<local-command-stdout>late</local-command-stdout>"},"timestamp":"2026-07-19T18:26:11Z"}`,
	}, "\n")
	s, err := Parse(strings.NewReader(lines), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 3 {
		t.Fatalf("got %d turns, want 3", len(s.Turns))
	}
	if out := s.Turns[0].Blocks[0].Command.Output; out != "" {
		t.Errorf("command turn Output = %q, want empty (stdout must not skip over a prompt)", out)
	}
	if blk := s.Turns[2].Blocks[0]; blk.Type != model.BlockText || blk.Text != "late" {
		t.Errorf("stdout turn = %q %q, want clean text turn", blk.Type, blk.Text)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/transcript/ -run 'TestParseCommand|TestParseSlashCommand|TestParseOrphanCommandOutput|TestParseCommandOutputNotAttached'`
Expected: FAIL to build — `undefined: parseCommand`, `model.BlockCommand` undefined.

- [ ] **Step 3: Implement**

1. In `internal/model/model.go`: add to the BlockType const block:

```go
	BlockCommand  BlockType = "command"
```

Add to the `Block` struct after the `Image` field:

```go
	Command *Command  // for BlockCommand
```

Add below the `Image` type:

```go
// Command is a slash-command invocation and its captured output.
type Command struct {
	Invocation string // "/model", "/code-review ultra 21"
	Output     string // local-command stdout/stderr text; may contain ANSI
}
```

2. Create `internal/transcript/command.go`:

```go
package transcript

import (
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// Slash commands appear in transcripts as two consecutive user records:
// an input record tagged <command-name>/<command-args> and an output
// record tagged <local-command-stdout>/<local-command-stderr>. This file
// recognizes both so the parser can merge them into one command turn.

// tagContent extracts the content of the first <tag>…</tag> pair in text.
func tagContent(text, tag string) (string, bool) {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(text, open)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// parseCommand extracts a slash-command invocation: the <command-name>
// content plus, when non-empty, a space and the <command-args> content.
// The <command-message> echo is dropped.
func parseCommand(text string) (string, bool) {
	name, ok := tagContent(text, "command-name")
	if !ok {
		return "", false
	}
	inv := strings.TrimSpace(name)
	if args, ok := tagContent(text, "command-args"); ok {
		if args = strings.TrimSpace(args); args != "" {
			inv += " " + args
		}
	}
	return inv, true
}

// parseCommandOutput extracts local-command output: stdout first, then
// stderr, joined with a newline when both are present.
func parseCommandOutput(text string) (string, bool) {
	out, okOut := tagContent(text, "local-command-stdout")
	errText, okErr := tagContent(text, "local-command-stderr")
	if !okOut && !okErr {
		return "", false
	}
	out, errText = strings.TrimSpace(out), strings.TrimSpace(errText)
	switch {
	case out != "" && errText != "":
		return out + "\n" + errText, true
	case errText != "":
		return errText, true
	default:
		return out, true
	}
}

// userText concatenates a turn's text-block contents.
func userText(t *model.Turn) string {
	var b strings.Builder
	for _, blk := range t.Blocks {
		if blk.Type == model.BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// isOpenCommand reports whether t is a slash-command turn still awaiting
// its output record.
func isOpenCommand(t *model.Turn) bool {
	return t.Kind == model.TurnUser && len(t.Blocks) == 1 &&
		t.Blocks[0].Type == model.BlockCommand && t.Blocks[0].Command.Output == ""
}
```

3. In `internal/transcript/parse.go`, the main-chain append currently reads:

```go
		// Main-chain records.
		turn := buildTurn(rec, blocks, toolIndex, seenUsage)
		if turn == nil {
			continue // e.g. a user record that only carried a tool_result
		}
		s.Turns = append(s.Turns, *turn)
```

Replace with:

```go
		// Main-chain records.
		turn := buildTurn(rec, blocks, toolIndex, seenUsage)
		if turn == nil {
			continue // e.g. a user record that only carried a tool_result
		}
		// Slash commands arrive as two user records (input, then output);
		// rewrite the input to a command block and absorb the output into it.
		if turn.Kind == model.TurnUser {
			text := userText(turn)
			if inv, ok := parseCommand(text); ok {
				turn.Blocks = []model.Block{{Type: model.BlockCommand, Command: &model.Command{Invocation: inv}}}
			} else if out, ok := parseCommandOutput(text); ok {
				if n := len(s.Turns); n > 0 && isOpenCommand(&s.Turns[n-1]) {
					s.Turns[n-1].Blocks[0].Command.Output = out
					continue
				}
				turn.Blocks = []model.Block{{Type: model.BlockText, Text: out}}
			}
		}
		s.Turns = append(s.Turns, *turn)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/transcript/ -race`
Expected: PASS (new tests plus all existing transcript tests unmodified).

- [ ] **Step 5: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS. (The render package does not yet know `BlockCommand` — its switches simply skip unknown block types, so nothing breaks before Task 2.)

- [ ] **Step 6: Commit**

```bash
git add internal/model/model.go internal/transcript/command.go internal/transcript/command_test.go internal/transcript/parse.go internal/transcript/parse_test.go
git commit -m "feat(transcript): merge slash-command input and output into one turn"
```

---

### Task 2: Command-card rendering

**Files:**
- Modify: `internal/render/render.go` (`renderTurnBody` switch, `turnPlainText` switch)
- Modify: `internal/render/assets/styles.css`
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `model.BlockCommand`, `model.Command{Invocation, Output string}`, `Block.Command` (Task 1); existing `StripANSI`, `renderTurnBody`, `turnPlainText`.
- Produces: `renderCommand(c *model.Command) string` (package render); CSS classes `command`, `command-invocation`, `command-output`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go`:

```go
func TestCommandCardRendersInvocationAndOutput(t *testing.T) {
	turn := model.Turn{Kind: model.TurnUser, Blocks: []model.Block{{
		Type:    model.BlockCommand,
		Command: &model.Command{Invocation: "/model", Output: "Set model to \x1b[1mFable 5\x1b[22m"},
	}}}
	got := string(renderTurnBody(turn, bodyCtx{links: newAgentLinks(nil, "")}))
	if !strings.Contains(got, `<code class="command-invocation">/model</code>`) {
		t.Errorf("invocation missing: %s", got)
	}
	if !strings.Contains(got, `<pre class="command-output">Set model to Fable 5</pre>`) {
		t.Errorf("output pre missing or ANSI not stripped: %s", got)
	}
}

func TestCommandCardWithoutOutputOmitsPre(t *testing.T) {
	turn := model.Turn{Kind: model.TurnUser, Blocks: []model.Block{{
		Type:    model.BlockCommand,
		Command: &model.Command{Invocation: "/clear"},
	}}}
	got := string(renderTurnBody(turn, bodyCtx{links: newAgentLinks(nil, "")}))
	if !strings.Contains(got, `<code class="command-invocation">/clear</code>`) {
		t.Errorf("invocation missing: %s", got)
	}
	if strings.Contains(got, "command-output") {
		t.Errorf("empty output must omit the pre block: %s", got)
	}
}

func TestCommandTurnPlainTextIsClean(t *testing.T) {
	turn := model.Turn{Kind: model.TurnUser, Blocks: []model.Block{{
		Type:    model.BlockCommand,
		Command: &model.Command{Invocation: "/model", Output: "Set model to \x1b[1mFable 5\x1b[22m"},
	}}}
	plain := turnPlainText(turn)
	if !strings.HasPrefix(plain, "/model") {
		t.Errorf("plain text = %q, want to start with the invocation", plain)
	}
	if !strings.Contains(plain, "Set model to Fable 5") {
		t.Errorf("plain text = %q, want ANSI-stripped output included", plain)
	}
	if strings.ContainsAny(plain, "<>") {
		t.Errorf("plain text = %q, must not contain markup", plain)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestCommandCard|TestCommandTurnPlainText'`
Expected: FAIL — the command block renders nothing (empty body, missing invocation markup).

- [ ] **Step 3: Implement**

1. In `internal/render/render.go`, add a case to the `renderTurnBody` switch (after the `BlockToolUse` case):

```go
		case model.BlockCommand:
			if blk.Command != nil {
				b.WriteString(renderCommand(blk.Command))
			}
```

2. Add below `renderTurnBody`:

```go
// renderCommand renders a slash-command invocation and its captured output
// as one block inside the user card.
func renderCommand(c *model.Command) string {
	var b strings.Builder
	b.WriteString(`<div class="command"><code class="command-invocation">` + html.EscapeString(c.Invocation) + `</code>`)
	if c.Output != "" {
		b.WriteString(`<pre class="command-output">` + html.EscapeString(StripANSI(c.Output)) + `</pre>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}
```

3. In `turnPlainText`, add a case to the switch (after the `BlockToolUse` case):

```go
		case model.BlockCommand:
			if blk.Command != nil {
				parts = append(parts, blk.Command.Invocation, blk.Command.Output)
			}
```

The function's trailing `StripANSI` already cleans the output for previews and search.

4. In `internal/render/assets/styles.css`, directly after the `.turn-body code` rule, insert:

```css
.command-invocation { display: inline-block; background: var(--code-bg); padding: .15rem .5rem; border-radius: 6px; font-size: .85rem; }
.command-output { color: var(--muted); }
```

(`.turn-body pre` already gives `.command-output` the monospace block styling.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/render/ -race`
Expected: PASS (new tests plus all existing render tests unmodified).

- [ ] **Step 5: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go internal/render/assets/styles.css
git commit -m "feat(render): render slash-command turns as one card"
```
