package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/redact"
	"github.com/saigyo/cc-what-have-i-done/internal/render"
	"github.com/saigyo/cc-what-have-i-done/internal/transcript"
)

func TestEndToEndRedactedReport(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s.jsonl")
	lines := strings.Join([]string{
		`{"type":"ai-title","aiTitle":"E2E"}`,
		`{"type":"user","message":{"role":"user","content":"my key is AKIAIOSFODNN7EXAMPLE"},"timestamp":"2026-07-11T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"ok"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]},"timestamp":"2026-07-11T10:00:01Z"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"a.txt"}]},"timestamp":"2026-07-11T10:00:02Z"}`,
	}, "\n")
	if err := os.WriteFile(src, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := transcript.ParseFile(src, transcript.Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	redact.Session(&sess, "")
	out := filepath.Join(dir, "report")
	if err := render.Site(sess, out, render.Options{}); err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
		t.Error("secret leaked into report")
	}
	if !strings.Contains(s, "REDACTED:aws-key") {
		t.Error("expected redaction marker in report")
	}
	if !strings.Contains(s, "a.txt") {
		t.Error("tool result missing from report")
	}
}
