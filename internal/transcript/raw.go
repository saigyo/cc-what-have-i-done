package transcript

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
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
	Source    *apiImageSource `json:"source"`
}

// apiImageSource mirrors the source object of an image content block.
type apiImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/png", …
	Data      string `json:"data"`
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

// toolResultParts flattens a tool_result content payload into plain text and
// any decodable images.
func toolResultParts(raw json.RawMessage) (string, []model.Image) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var blocks []apiBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", nil
	}
	var parts []string
	var images []model.Image
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
		if img, ok := decodeImage(b.Source); ok {
			images = append(images, img)
		}
	}
	return strings.Join(parts, "\n"), images
}

// imageMediaTypes is the set of media types Claude accepts; parsing admits
// only these. The renderer maps them to file extensions.
var imageMediaTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true,
}

// decodeImage decodes an image content block's source. ok is false — and the
// block is skipped — for missing or non-base64 sources, unsupported media
// types, and data that does not decode to at least one byte.
func decodeImage(src *apiImageSource) (model.Image, bool) {
	if src == nil || src.Type != "base64" || !imageMediaTypes[src.MediaType] {
		return model.Image{}, false
	}
	data, err := base64.StdEncoding.DecodeString(src.Data)
	if err != nil || len(data) == 0 {
		return model.Image{}, false
	}
	return model.Image{MediaType: src.MediaType, Data: data}, true
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
