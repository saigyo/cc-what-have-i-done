package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// NOTE: deviates from the task brief's literal fixture, which used
	// "/Users/markus/IdeaProjects/cc-what-have-i-done" (this repo's own
	// path). That path contains a literal hyphen in the directory name
	// ("cc-what-have-i-done"), which the naive '/'<->'-' replace scheme
	// specified below cannot round-trip: every embedded hyphen gets
	// decoded back into a '/'. This is a mathematical property of the
	// scheme, not an implementation bug, so a hyphen-free path is used
	// here instead to exercise a genuinely invertible case. See the
	// task report for verification (go run of the exact replace calls
	// against the original fixture).
	cwd := "/Users/markus/IdeaProjects/ccwhid"
	enc := EncodeProjectDir(cwd)
	if enc != "-Users-markus-IdeaProjects-ccwhid" {
		t.Fatalf("EncodeProjectDir = %q", enc)
	}
	if dec := DecodeProjectDir(enc); dec != cwd {
		t.Fatalf("DecodeProjectDir = %q, want %q", dec, cwd)
	}
}

// writeSession writes a minimal valid jsonl file with the given lines.
func writeSession(t *testing.T, dir, id string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".jsonl")
	data := ""
	for _, l := range lines {
		data += l + "\n"
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	s := strings.Repeat("ä", 80)
	got := truncate(s, 60)
	if !utf8.ValidString(got) {
		t.Errorf("truncate produced invalid UTF-8: %q", got)
	}
}

func TestScanBuildsGroups(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-tmp-projA")
	writeSession(t, proj, "aaaa1111-0000-0000-0000-000000000000",
		`{"type":"ai-title","aiTitle":"Build a parser"}`,
		`{"type":"user","message":{"role":"user","content":"hello"},"timestamp":"2026-07-11T10:00:00Z"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]},"timestamp":"2026-07-11T10:00:01Z"}`,
	)
	groups, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	g := groups[0]
	if g.ProjectPath != "/tmp/projA" {
		t.Errorf("ProjectPath = %q", g.ProjectPath)
	}
	if len(g.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(g.Sessions))
	}
	s := g.Sessions[0]
	if s.Title != "Build a parser" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.MessageCount != 2 {
		t.Errorf("MessageCount = %d, want 2", s.MessageCount)
	}
}

func TestFindSessionByPrefix(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-tmp-projA")
	writeSession(t, proj, "abcd1111-0000-0000-0000-000000000000",
		`{"type":"user","message":{"role":"user","content":"x"},"timestamp":"2026-07-11T10:00:00Z"}`)
	writeSession(t, proj, " zzzz2222")
	got, err := FindSession(root, "abcd")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "abcd1111-0000-0000-0000-000000000000" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestScanUsesTranscriptCwd(t *testing.T) {
	root := t.TempDir()
	// Dir name would decode losslessly-wrong; the transcript cwd is authoritative.
	proj := filepath.Join(root, "-tmp-proj-with-hyphen")
	writeSession(t, proj, "cccc3333-0000-0000-0000-000000000000",
		`{"type":"user","message":{"role":"user","content":"hi"},"cwd":"/tmp/proj-with-hyphen","timestamp":"2026-07-11T10:00:00Z"}`)
	groups, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}
	if groups[0].ProjectPath != "/tmp/proj-with-hyphen" {
		t.Errorf("ProjectPath = %q, want /tmp/proj-with-hyphen (from transcript cwd)", groups[0].ProjectPath)
	}
	if groups[0].Sessions[0].ProjectPath != "/tmp/proj-with-hyphen" {
		t.Errorf("session ProjectPath = %q, want the true cwd", groups[0].Sessions[0].ProjectPath)
	}
}

func TestScanClassifiesAgentSessions(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-tmp-projA")
	// Interactive session: entrypoint cli, no promptSource.
	writeSession(t, proj, "aaaa1111-0000-0000-0000-000000000000",
		`{"type":"user","message":{"role":"user","content":"hi"},"entrypoint":"cli","cwd":"/tmp/projA","timestamp":"2026-07-11T10:00:00Z"}`)
	// Agent session: entrypoint sdk-py + promptSource sdk.
	writeSession(t, proj, "bbbb2222-0000-0000-0000-000000000000",
		`{"type":"user","message":{"role":"user","content":"review"},"entrypoint":"sdk-py","promptSource":"sdk","cwd":"/tmp/projA","timestamp":"2026-07-11T11:00:00Z"}`)
	groups, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]SessionInfo{}
	for _, s := range groups[0].Sessions {
		byID[s.ID[:4]] = s
	}
	if byID["aaaa"].IsAgent {
		t.Error("cli session wrongly classified as agent")
	}
	if !byID["bbbb"].IsAgent {
		t.Error("sdk session not classified as agent")
	}
	if got := groups[0].AgentCount(); got != 1 {
		t.Errorf("AgentCount = %d, want 1", got)
	}
	if got := len(groups[0].RootSessions()); got != 1 {
		t.Errorf("RootSessions = %d, want 1", got)
	}
	if root, agent := groups[0].Counts(); root != 1 || agent != 1 {
		t.Errorf("Counts = (%d, %d), want (1, 1)", root, agent)
	}
}

func TestFindProject(t *testing.T) {
	groups := []ProjectGroup{
		{ProjectPath: "/Users/x/IdeaProjects/cc-what-have-i-done"},
		{ProjectPath: "/Users/x/IdeaProjects/other-app"},
		{ProjectPath: "/Users/x/Downloads/apparatus"},
	}
	cases := []struct {
		want    string
		wantIdx int
		wantErr bool
	}{
		{"/Users/x/IdeaProjects/other-app", 1, false}, // exact path
		{"cc-what-have-i-done", 0, false},             // basename
		{"OTHER-APP", 1, false},                       // basename, case-insensitive
		{"Downloads", 2, false},                       // substring
		{"nope", -1, true},                            // no match
		{"IdeaProjects", -1, true},                    // ambiguous substring
	}
	for _, c := range cases {
		idx, err := FindProject(groups, c.want)
		if c.wantErr {
			if err == nil {
				t.Errorf("FindProject(%q) = %d, want error", c.want, idx)
			}
			continue
		}
		if err != nil || idx != c.wantIdx {
			t.Errorf("FindProject(%q) = %d, %v; want %d", c.want, idx, err, c.wantIdx)
		}
	}
}

func TestDisplayLabel(t *testing.T) {
	if got := (SessionInfo{Title: "T"}).DisplayLabel(); got != "T" {
		t.Errorf("DisplayLabel = %q", got)
	}
	if got := (SessionInfo{ID: "id"}).DisplayLabel(); got != "id" {
		t.Errorf("DisplayLabel fallback = %q", got)
	}
}
