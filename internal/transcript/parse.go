package transcript

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// Options controls how a transcript is folded into a Session.
type Options struct {
	IncludeSubagents bool
	// AgentFile marks the input as an agent's own transcript file: its records
	// carry isSidechain=true but form the file's main chain, so the flag is
	// ignored.
	AgentFile bool
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
	// seenUsage tracks message ids whose usage has already been counted, so an
	// assistant message split across multiple records is not counted per-record.
	seenUsage := map[string]bool{}
	// lastTask tracks the most recent Task tool call; sidechainOwner is the Task
	// whose Subagents any subsequent sidechain turns are appended to.
	var lastTask *model.ToolCall
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
		if opts.AgentFile {
			rec.IsSidechain = false
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
			turn := buildTurn(rec, blocks, toolIndex, seenUsage)
			if turn == nil {
				continue
			}
			sub := &sidechainOwner.Subagents[len(sidechainOwner.Subagents)-1]
			sub.Turns = append(sub.Turns, *turn)
			continue
		}

		// Main-chain records.
		turn := buildTurn(rec, blocks, toolIndex, seenUsage)
		if turn == nil {
			continue // e.g. a user record that only carried a tool_result
		}
		s.Turns = append(s.Turns, *turn)

		// Seed a subagent slot for each Task tool call and make it the current
		// sidechain owner. Tool-use ids are already registered in buildTurn.
		// Note: sidechain records are attributed to the nearest preceding Task
		// by file order. Sequential Task calls attribute correctly; multiple
		// Task calls dispatched in parallel from one assistant message cannot be
		// disambiguated from the raw records and will all attach to the last
		// one — a known v1 limitation.
		if opts.IncludeSubagents {
			for i := range turn.Blocks {
				tool := turn.Blocks[i].Tool
				if tool != nil && tool.Name == "Task" {
					tool.Subagents = append(tool.Subagents, model.Subagent{
						Description: taskDescription(tool),
					})
					lastTask = tool
					sidechainOwner = lastTask
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		// A single over-long line (bufio.ErrTooLong) is a pathological record,
		// not a reason to discard the whole report: count it and return what we
		// parsed so far.
		if errors.Is(err, bufio.ErrTooLong) {
			s.SkippedLines++
			return s, nil
		}
		return s, err
	}
	return s, nil
}

// messageMeta converts a record's message model/usage into turn fields, also
// returning the message id for deduplication. Only assistant records carry
// usage; the aggregate cache_creation figure (with no 5m/1h split) is attributed
// to the cheaper 5-minute write.
func messageMeta(raw json.RawMessage) (msgID, modelID string, usage *model.Usage) {
	id, mdl, u := decodeMessageMeta(raw)
	if u == nil {
		return id, mdl, nil
	}
	w5, w1 := u.CacheCreation.Ephemeral5m, u.CacheCreation.Ephemeral1h
	if w5 == 0 && w1 == 0 && u.CacheCreationInputTokens > 0 {
		w5 = u.CacheCreationInputTokens
	}
	return id, mdl, &model.Usage{
		Input:        u.InputTokens,
		Output:       u.OutputTokens,
		CacheRead:    u.CacheReadInputTokens,
		CacheWrite5m: w5,
		CacheWrite1h: w1,
	}
}

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
	t := strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(t, "<task-notification>") {
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

// tagContent returns the text between the first <name> and its closing
// </name>, trimmed. Simple fields match the first closing tag; <result> keeps
// matching the last one so bodies that quote XML-looking text (even a literal
// </result>) stay intact.
func tagContent(s, name string) string {
	open, close := "<"+name+">", "</"+name+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	rest := s[i+len(open):]
	var j int
	if name == "result" {
		j = strings.LastIndex(rest, close)
	} else {
		j = strings.Index(rest, close)
	}
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

// buildTurn converts a record's blocks into a model.Turn. tool_result blocks are
// attached to their originating ToolCall via toolIndex and do not themselves
// produce turn content. Returns nil if the record yields no displayable blocks.
func buildTurn(rec rawRecord, blocks []apiBlock, toolIndex map[string]*model.ToolCall, seenUsage map[string]bool) *model.Turn {
	turn := &model.Turn{Timestamp: parseTime(rec.Timestamp)}
	if rec.Type == "user" {
		turn.Kind = model.TurnUser
	} else {
		turn.Kind = model.TurnAssistant
	}
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
			tc := buildToolCall(b)
			// Register the tool call so a later tool_result (in this turn's
			// chain OR inside a sidechain) can be matched back to it by id.
			toolIndex[tc.ID] = tc
			turn.Blocks = append(turn.Blocks, model.Block{Type: model.BlockToolUse, Tool: tc})
		case "tool_result":
			if tc := toolIndex[b.ToolUseID]; tc != nil {
				tc.Result = &model.ToolResult{Content: toolResultText(b.Content), IsError: b.IsError}
			}
		}
	}
	if len(turn.Blocks) == 0 {
		return nil
	}
	if rec.Type == "assistant" {
		msgID, mdl, u := messageMeta(rec.Message)
		turn.Model = mdl
		// One assistant message is written as several records (one per content
		// block), each repeating the same usage. Count usage on the first record
		// of a message id only; later records of the same message get nil usage
		// so totals are not multiplied by the block count.
		if u != nil && msgID != "" {
			if seenUsage[msgID] {
				u = nil
			} else {
				seenUsage[msgID] = true
			}
		}
		turn.Usage = u
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
	if tc.IsAgent() {
		tc.AgentPrompt = str(input, "prompt")
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
	case "Task", "Agent":
		return str(in, "description")
	case "Skill":
		return str(in, "skill")
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
