package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

func TestResolveOutDirDefault(t *testing.T) {
	opts := &options{}
	si := discovery.SessionInfo{ID: "abcd1234-5678-90ef-0000-000000000000"}
	got := resolveOutDir(opts, si)
	if got != filepath.Join("ccwhid-report", "abcd1234") {
		t.Errorf("resolveOutDir = %q", got)
	}
}

func TestGenerateProducesReport(t *testing.T) {
	// Build a fake transcript file.
	dir := t.TempDir()
	src := filepath.Join(dir, "sess.jsonl")
	lines := strings.Join([]string{
		`{"type":"ai-title","aiTitle":"Test run"}`,
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-07-11T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"world"}]},"timestamp":"2026-07-11T10:00:01Z"}`,
	}, "\n")
	if err := os.WriteFile(src, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "report")
	opts := &options{out: out, includeSubagents: true}
	si := discovery.SessionInfo{ID: "sess", FilePath: src}
	got, err := generate(opts, si)
	if err != nil {
		t.Fatal(err)
	}
	if got != out {
		t.Errorf("generate returned %q, want %q", got, out)
	}
	html, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "Test run") || !strings.Contains(string(html), "world") {
		t.Error("report missing expected content")
	}
}
