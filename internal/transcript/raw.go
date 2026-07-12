package transcript

import (
	"encoding/json"
	"strings"
	"time"
)

// rawRecord is one decoded line of a session .jsonl file.
type rawRecord struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	Timestamp   string          `json:"timestamp"`
	GitBranch   string          `json:"gitBranch"`
	Version     string          `json:"version"`
	Cwd         string          `json:"cwd"`
	AiTitle     string          `json:"aiTitle"`
	IsSidechain bool            `json:"isSidechain"`
	IsMeta      bool            `json:"isMeta"`
	Message     json.RawMessage `json:"message"`
}

// apiBlock is one content block of an Anthropic message.
type apiBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

// decodeMessageContent extracts the role and content blocks from a raw message
// object. Content may be a plain string (wrapped as a single text block) or an
// array of blocks.
func decodeMessageContent(raw json.RawMessage) (string, []apiBlock) {
	if len(raw) == 0 {
		return "", nil
	}
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return "", nil
	}
	var s string
	if json.Unmarshal(msg.Content, &s) == nil {
		return msg.Role, []apiBlock{{Type: "text", Text: s}}
	}
	var blocks []apiBlock
	_ = json.Unmarshal(msg.Content, &blocks) // tolerate; empty on failure
	return msg.Role, blocks
}

// toolResultText flattens a tool_result content payload to plain text.
func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// apiUsage mirrors the message.usage object of an assistant record.
type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheCreation            struct {
		Ephemeral5m int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// decodeMessageMeta extracts the message id, model id, and usage (if any) from a
// raw message. The message id lets callers deduplicate usage: Claude Code splits
// one assistant message across several records (one per content block), each
// repeating the same usage object, so usage must be counted once per message id.
func decodeMessageMeta(raw json.RawMessage) (msgID, modelID string, usage *apiUsage) {
	if len(raw) == 0 {
		return "", "", nil
	}
	var m struct {
		ID    string    `json:"id"`
		Model string    `json:"model"`
		Usage *apiUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", "", nil
	}
	return m.ID, m.Model, m.Usage
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
