package transcript

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestDecodeMessageContentString(t *testing.T) {
	role, blocks := decodeMessageContent([]byte(`{"role":"user","content":"hello world"}`))
	if role != "user" {
		t.Errorf("role = %q", role)
	}
	if len(blocks) != 1 || blocks[0].Type != "text" || blocks[0].Text != "hello world" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestDecodeMessageContentArray(t *testing.T) {
	raw := `{"role":"assistant","content":[
		{"type":"thinking","thinking":"hmm"},
		{"type":"text","text":"done"},
		{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}
	]}`
	_, blocks := decodeMessageContent([]byte(raw))
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks", len(blocks))
	}
	if blocks[2].Name != "Bash" {
		t.Errorf("tool name = %q", blocks[2].Name)
	}
}

// tinyPNGb64 is a valid 1×1 PNG, base64-encoded.
const tinyPNGb64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestDecodeImage(t *testing.T) {
	img, ok := decodeImage(&apiImageSource{Type: "base64", MediaType: "image/png", Data: tinyPNGb64})
	if !ok || img.MediaType != "image/png" {
		t.Fatalf("decodeImage(valid) = %+v, %v", img, ok)
	}
	want, err := base64.StdEncoding.DecodeString(tinyPNGb64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(img.Data, want) {
		t.Error("decoded bytes mismatch")
	}
	for name, src := range map[string]*apiImageSource{
		"nil":          nil,
		"not-base64":   {Type: "url", MediaType: "image/png", Data: tinyPNGb64},
		"bad-media":    {Type: "base64", MediaType: "image/tiff", Data: tinyPNGb64},
		"corrupt-data": {Type: "base64", MediaType: "image/png", Data: "%%%not-base64%%%"},
		"empty-data":   {Type: "base64", MediaType: "image/png", Data: ""},
	} {
		if _, ok := decodeImage(src); ok {
			t.Errorf("decodeImage(%s) unexpectedly ok", name)
		}
	}
}

func TestToolResultParts(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"a"},` +
		`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + tinyPNGb64 + `"}},` +
		`{"type":"text","text":"b"}]`)
	text, images := toolResultParts(raw)
	if text != "a\nb" {
		t.Errorf("text = %q, want %q", text, "a\nb")
	}
	if len(images) != 1 || images[0].MediaType != "image/png" || len(images[0].Data) == 0 {
		t.Fatalf("images = %+v", images)
	}
	// Plain-string payloads keep working.
	text, images = toolResultParts(json.RawMessage(`"plain"`))
	if text != "plain" || images != nil {
		t.Errorf("plain string: got %q, %v", text, images)
	}
	// Undecodable image blocks are dropped; text survives.
	raw = json.RawMessage(`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"%%%"}},{"type":"text","text":"x"}]`)
	text, images = toolResultParts(raw)
	if text != "x" || len(images) != 0 {
		t.Errorf("corrupt image: got %q, %v", text, images)
	}
}
