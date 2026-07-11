package transcript

import "testing"

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

func TestToolResultTextArray(t *testing.T) {
	got := toolResultText([]byte(`[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]`))
	if got != "line1\nline2" {
		t.Errorf("toolResultText = %q", got)
	}
}

func TestToolResultTextString(t *testing.T) {
	if got := toolResultText([]byte(`"just text"`)); got != "just text" {
		t.Errorf("toolResultText = %q", got)
	}
}
