// Package model defines the domain types shared across ccwhid: a parsed
// Session and the timeline of Turns, Blocks and ToolCalls within it.
package model

import "time"

type TurnKind string

const (
	TurnUser        TurnKind = "user"
	TurnAssistant   TurnKind = "assistant"
	TurnAgentResult TurnKind = "agent-result" // background-agent completion fed back into the session
)

type BlockType string

const (
	BlockText     BlockType = "text"
	BlockThinking BlockType = "thinking"
	BlockToolUse  BlockType = "tool_use"
	BlockImage    BlockType = "image"
)

// Session is one fully-parsed Claude Code transcript.
type Session struct {
	ID           string
	ProjectPath  string // decoded absolute cwd
	Title        string
	GitBranch    string
	Version      string
	StartedAt    time.Time
	EndedAt      time.Time
	Turns        []Turn
	SkippedLines int            // count of malformed lines skipped while parsing
	Agents       []AgentSession // linked subagent sessions (subagents/ dir), when loaded
}

func (s Session) Duration() time.Duration { return s.EndedAt.Sub(s.StartedAt) }

func (s Session) DisplayTitle() string {
	if s.Title != "" {
		return s.Title
	}
	return "Untitled session"
}

// Turn is a single user or assistant message, holding ordered content blocks.
type Turn struct {
	Kind      TurnKind
	Timestamp time.Time
	Blocks    []Block
	Model     string // assistant model id (empty for user turns / when absent)
	Usage     *Usage // token usage for this assistant turn; nil when absent
	// Agent-result fields, set only when Kind == TurnAgentResult.
	AgentID      string // task/agent id from the notification
	AgentStatus  string // e.g. "completed"
	AgentSummary string // e.g. `Agent "Implement Task 12" finished`
}

// Usage holds the token counts reported for one assistant message. Cache writes
// keep the 5-minute / 1-hour split because they are priced differently.
type Usage struct {
	Input        int
	Output       int
	CacheRead    int
	CacheWrite5m int
	CacheWrite1h int
}

// Block is one content unit: assistant/user text, a thinking block, a tool
// call, or an image pasted into the conversation.
type Block struct {
	Type  BlockType
	Text  string    // for BlockText and BlockThinking
	Tool  *ToolCall // for BlockToolUse
	Image *Image    // for BlockImage
}

// Image is one decoded image from the transcript — pasted by the user or
// returned inside a tool result. Data holds raw bytes, not base64.
type Image struct {
	MediaType string // e.g. "image/png"
	Data      []byte
}

// ToolCall is a single tool invocation and its result.
type ToolCall struct {
	ID          string
	Name        string     // e.g. "Bash", "Edit", "Read", "Task"
	Summary     string     // one-line summary for the collapsed card header
	InputJSON   string     // pretty-printed input for generic display
	AgentPrompt string     // for Task/Agent calls: the subagent prompt, rendered as markdown
	Description string     // for TaskCreate calls: the task description, rendered as markdown
	TaskNumber  string     // for TaskCreate calls: the created task's number from the result, e.g. "12"
	Questions   []Question // set for AskUserQuestion calls
	Result      *ToolResult
	Diff        *Diff      // set for Edit/Write
	Subagents   []Subagent // set for Task calls with sidechain activity
}

// IsAgent reports whether this tool call spawns a subagent — the historical
// "Task" tool or the modern "Agent" tool.
func (t *ToolCall) IsAgent() bool {
	return t.Name == "Task" || t.Name == "Agent"
}

// IsAskUserQuestion reports whether this tool call is an AskUserQuestion prompt.
func (t *ToolCall) IsAskUserQuestion() bool {
	return t.Name == "AskUserQuestion"
}

// IsTaskCreate reports whether this tool call creates a tracked task.
func (t *ToolCall) IsTaskCreate() bool {
	return t.Name == "TaskCreate"
}

// IsTaskUpdate reports whether this tool call updates a tracked task.
func (t *ToolCall) IsTaskUpdate() bool {
	return t.Name == "TaskUpdate"
}

// Question is one question posed by an AskUserQuestion call, with its options.
type Question struct {
	Header      string // short chip label, e.g. "Language"
	Prompt      string // the full question text
	MultiSelect bool
	Options     []QuestionOption
}

// QuestionOption is one selectable answer of a Question.
type QuestionOption struct {
	Label       string
	Description string
	Preview     string // optional monospace preview block
}

type ToolResult struct {
	Content string
	IsError bool
	Images  []Image // images embedded in the result's content array
}

type Diff struct {
	Path    string
	OldText string
	NewText string
}

// Subagent is a nested sidechain run attached to a parent Task tool call.
type Subagent struct {
	Description string
	Turns       []Turn
}

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
