package transcript

import (
	"bufio"
	"encoding/json"
	"io"
	"os"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// Options controls how a transcript is folded into a Session.
type Options struct {
	IncludeSubagents bool
}

// ParseFile parses a transcript file at path.
func ParseFile(path string, opts Options) (model.Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.Session{}, err
	}
	defer f.Close()
	s, err := Parse(f, opts)
	if err != nil {
		return model.Session{}, err
	}
	s.ID = sessionIDFromPath(path)
	return s, nil
}

func sessionIDFromPath(path string) string {
	base := path
	if i := lastSlash(path); i >= 0 {
		base = path[i+1:]
	}
	if len(base) > len(".jsonl") && base[len(base)-len(".jsonl"):] == ".jsonl" {
		return base[:len(base)-len(".jsonl")]
	}
	return base
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// Parse reads a transcript stream and folds it into a Session timeline.
func Parse(r io.Reader, opts Options) (model.Session, error) {
	var s model.Session
	// toolIndex maps a tool_use id to the ToolCall pointer so results and
	// sidechains can be attached after the fact.
	toolIndex := map[string]*model.ToolCall{}
	// lastTask tracks the most recent Task tool call for sidechain attachment.
	var lastTask *model.ToolCall
	// subByFirstUUID lets sidechain turns append to the right subagent; keyed
	// by the Task tool that owns them.
	sidechainOwner := lastTask

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(trimSpace(line)) == 0 {
			continue
		}
		var rec rawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			s.SkippedLines++
			continue
		}
		switch rec.Type {
		case "ai-title":
			if rec.AiTitle != "" {
				s.Title = rec.AiTitle
			}
			continue
		case "user", "assistant":
			// fallthrough to handling below
		default:
			continue // ignore mode/permission/attachment/etc. for the timeline
		}
		if rec.IsMeta {
			continue
		}

		// Capture session-level metadata from the first record that has it.
		if s.GitBranch == "" && rec.GitBranch != "" {
			s.GitBranch = rec.GitBranch
		}
		if s.Version == "" && rec.Version != "" {
			s.Version = rec.Version
		}
		if s.ProjectPath == "" && rec.Cwd != "" {
			s.ProjectPath = rec.Cwd
		}
		ts := parseTime(rec.Timestamp)
		if !ts.IsZero() {
			if s.StartedAt.IsZero() {
				s.StartedAt = ts
			}
			s.EndedAt = ts
		}

		_, blocks := decodeMessageContent(rec.Message)

		// Sidechain (subagent) records.
		if rec.IsSidechain {
			if !opts.IncludeSubagents || sidechainOwner == nil {
				continue
			}
			turn := buildTurn(rec, blocks, toolIndex)
			if turn == nil {
				continue
			}
			sub := &sidechainOwner.Subagents[len(sidechainOwner.Subagents)-1]
			sub.Turns = append(sub.Turns, *turn)
			continue
		}

		// Main-chain records.
		turn := buildTurn(rec, blocks, toolIndex)
		if turn == nil {
			continue // e.g. a user record that only carried a tool_result
		}
		s.Turns = append(s.Turns, *turn)

		// After appending, register any Task tool call as the new sidechain
		// owner and seed its first subagent slot.
		for i := range turn.Blocks {
			b := &s.Turns[len(s.Turns)-1].Blocks[i]
			if b.Tool != nil {
				toolIndex[b.Tool.ID] = b.Tool
				if b.Tool.Name == "Task" && opts.IncludeSubagents {
					b.Tool.Subagents = append(b.Tool.Subagents, model.Subagent{
						Description: taskDescription(b.Tool),
					})
					lastTask = b.Tool
					sidechainOwner = lastTask
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// buildTurn converts a record's blocks into a model.Turn. tool_result blocks are
// attached to their originating ToolCall via toolIndex and do not themselves
// produce turn content. Returns nil if the record yields no displayable blocks.
func buildTurn(rec rawRecord, blocks []apiBlock, toolIndex map[string]*model.ToolCall) *model.Turn {
	turn := &model.Turn{Timestamp: parseTime(rec.Timestamp)}
	if rec.Type == "user" {
		turn.Kind = model.TurnUser
	} else {
		turn.Kind = model.TurnAssistant
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				turn.Blocks = append(turn.Blocks, model.Block{Type: model.BlockText, Text: b.Text})
			}
		case "thinking":
			if b.Thinking != "" {
				turn.Blocks = append(turn.Blocks, model.Block{Type: model.BlockThinking, Text: b.Thinking})
			}
		case "tool_use":
			turn.Blocks = append(turn.Blocks, model.Block{Type: model.BlockToolUse, Tool: buildToolCall(b)})
		case "tool_result":
			if tc := toolIndex[b.ToolUseID]; tc != nil {
				tc.Result = &model.ToolResult{Content: toolResultText(b.Content), IsError: b.IsError}
			}
		}
	}
	if len(turn.Blocks) == 0 {
		return nil
	}
	return turn
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}

// buildToolCall constructs a ToolCall from a tool_use block, deriving a summary
// and, for Edit/Write, a Diff.
func buildToolCall(b apiBlock) *model.ToolCall {
	tc := &model.ToolCall{
		ID:        b.ID,
		Name:      b.Name,
		InputJSON: prettyJSON(b.Input),
	}
	input := decodeInput(b.Input)
	tc.Summary = toolSummary(b.Name, input)
	switch b.Name {
	case "Edit", "Write":
		tc.Diff = buildDiff(b.Name, input)
	}
	return tc
}

func decodeInput(raw json.RawMessage) map[string]any {
	m := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

func str(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// toolSummary produces a compact one-line header for a tool card.
func toolSummary(name string, in map[string]any) string {
	switch name {
	case "Bash":
		if c := str(in, "command"); c != "" {
			return firstLine(c)
		}
	case "Read", "Edit", "Write", "NotebookEdit":
		if p := str(in, "file_path"); p != "" {
			return p
		}
	case "Glob":
		return str(in, "pattern")
	case "Grep":
		return str(in, "pattern")
	case "Task":
		return str(in, "description")
	case "WebFetch":
		return str(in, "url")
	case "WebSearch":
		return str(in, "query")
	}
	return ""
}

func buildDiff(name string, in map[string]any) *model.Diff {
	d := &model.Diff{Path: str(in, "file_path")}
	if name == "Write" {
		d.NewText = str(in, "content")
		return d
	}
	d.OldText = str(in, "old_string")
	d.NewText = str(in, "new_string")
	return d
}

func taskDescription(tc *model.ToolCall) string {
	if tc.Summary != "" {
		return tc.Summary
	}
	return "subagent"
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(out)
}
