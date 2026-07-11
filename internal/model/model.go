// Package model defines the domain types shared across ccwhid: a parsed
// Session and the timeline of Turns, Blocks and ToolCalls within it.
package model

import "time"

type TurnKind string

const (
	TurnUser      TurnKind = "user"
	TurnAssistant TurnKind = "assistant"
)

type BlockType string

const (
	BlockText     BlockType = "text"
	BlockThinking BlockType = "thinking"
	BlockToolUse  BlockType = "tool_use"
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
	SkippedLines int // count of malformed lines skipped while parsing
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
}

// Block is one content unit: assistant/user text, a thinking block, or a tool call.
type Block struct {
	Type BlockType
	Text string    // for BlockText and BlockThinking
	Tool *ToolCall // for BlockToolUse
}

// ToolCall is a single tool invocation and its result.
type ToolCall struct {
	ID        string
	Name      string // e.g. "Bash", "Edit", "Read", "Task"
	Summary   string // one-line summary for the collapsed card header
	InputJSON string // pretty-printed input for generic display
	Result    *ToolResult
	Diff      *Diff      // set for Edit/Write
	Subagents []Subagent // set for Task calls with sidechain activity
}

type ToolResult struct {
	Content string
	IsError bool
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
