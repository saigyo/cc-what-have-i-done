package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saigyo/cc-what-have-i-done/internal/discovery"
)

// writeLatestSession writes a one-line transcript with a controlled mtime so
// latestSession's newest-first selection can be exercised deterministically.
func writeLatestSession(t *testing.T, root, projDir, id, entrypoint, promptSource string, mod time.Time) {
	t.Helper()
	dir := filepath.Join(root, projDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".jsonl")
	line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"x"},"entrypoint":%q,"promptSource":%q,"cwd":%q,"timestamp":"2026-07-11T10:00:00Z"}`,
		entrypoint, promptSource, "/"+projDir)
	if err := os.WriteFile(p, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestLatestSessionPicksNewestInteractive(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	// Project A: newest file is an agent transcript; its interactive one is older.
	writeLatestSession(t, root, "-a", "aaaaaaaa-0000-0000-0000-000000000000", "sdk-py", "sdk", base.Add(100*time.Minute))
	writeLatestSession(t, root, "-a", "aaaaaaaa-1111-0000-0000-000000000000", "cli", "", base.Add(50*time.Minute))
	// Project B: interactive, newer than A's interactive but older than A's agent.
	writeLatestSession(t, root, "-b", "bbbbbbbb-0000-0000-0000-000000000000", "cli", "", base.Add(90*time.Minute))

	// Unscoped: must pick the globally newest interactive session (B), not A's
	// older interactive session, and never the agent transcript.
	si, needTUI, err := latestSession(&options{latest: true}, root)
	if err != nil || needTUI {
		t.Fatalf("latestSession err=%v needTUI=%v", err, needTUI)
	}
	if si.IsAgent {
		t.Fatal("latestSession returned an agent transcript")
	}
	if !strings.HasPrefix(si.ID, "bbbbbbbb") {
		t.Errorf("unscoped --latest picked %q, want newest interactive bbbbbbbb...", si.ID)
	}

	// Scoped to project A: must skip the newer agent and return A's interactive.
	si, _, err = latestSession(&options{latest: true, project: "-a"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(si.ID, "aaaaaaaa-1111") {
		t.Errorf("--project -a --latest picked %q, want aaaaaaaa-1111...", si.ID)
	}
}

func TestLatestSessionNoInteractiveErrors(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	writeLatestSession(t, root, "-a", "aaaaaaaa-0000-0000-0000-000000000000", "sdk-py", "sdk", base)
	if _, _, err := latestSession(&options{latest: true, project: "-a"}, root); err == nil {
		t.Error("expected error when project has only agent sessions")
	}
}

func TestEnsureOutDir(t *testing.T) {
	// Non-existent dir is created.
	base := t.TempDir()
	fresh := filepath.Join(base, "new")
	if err := ensureOutDir(fresh, false); err != nil {
		t.Fatalf("fresh dir: %v", err)
	}
	if fi, err := os.Stat(fresh); err != nil || !fi.IsDir() {
		t.Fatalf("expected created dir, got %v %v", fi, err)
	}

	// Non-empty dir without --force errors.
	nonEmpty := t.TempDir()
	stale := filepath.Join(nonEmpty, "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureOutDir(nonEmpty, false); err == nil {
		t.Error("expected error for non-empty dir without force")
	}

	// Non-empty dir with --force clears stale content (incl. nested).
	nested := filepath.Join(nonEmpty, "assets")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "old.css"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureOutDir(nonEmpty, true); err != nil {
		t.Fatalf("force clear: %v", err)
	}
	entries, err := os.ReadDir(nonEmpty)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("force did not clear dir, %d entries remain", len(entries))
	}
}

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
