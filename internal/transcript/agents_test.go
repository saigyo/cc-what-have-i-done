package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// agentRecord is a minimal agent-file line: sidechain-flagged user text.
const agentRecord = `{"type":"user","isSidechain":true,"timestamp":"2026-07-05T18:11:19.567Z","message":{"role":"user","content":"do the thing"}}`

func writeAgentFixture(t *testing.T) (rootPath string) {
	t.Helper()
	dir := t.TempDir()
	rootPath = filepath.Join(dir, "root-session.jsonl")
	if err := os.WriteFile(rootPath, []byte(`{"type":"user","message":{"role":"user","content":"hi"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "root-session", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"agent-bbb222.jsonl":     agentRecord + "\n",
		"agent-aaa111.jsonl":     agentRecord + "\n",
		"agent-aaa111.meta.json": `{"agentType":"general-purpose","description":"Implement Task 12","toolUseId":"toolu_01x","spawnDepth":1}`,
		"agent-broken.jsonl":     "", // empty file parses to an empty session, still listed
		"notes.txt":              "ignore me",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(sub, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return rootPath
}

func TestLoadAgentSessions(t *testing.T) {
	root := writeAgentFixture(t)
	agents, err := LoadAgentSessions(root, Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(agents))
	}
	// Sorted by file name: aaa111, bbb222, broken.
	a := agents[0]
	if a.ID != "aaa111" || a.Description != "Implement Task 12" || a.AgentType != "general-purpose" || a.ToolUseID != "toolu_01x" || a.SpawnDepth != 1 {
		t.Errorf("meta not applied: %+v", a)
	}
	if len(a.Session.Turns) != 1 || a.Session.Turns[0].Kind != model.TurnUser {
		t.Errorf("agent transcript not parsed as main chain: %+v", a.Session.Turns)
	}
	// No meta.json -> Description falls back to ID.
	if agents[1].ID != "bbb222" || agents[1].Description != "bbb222" {
		t.Errorf("fallback description wrong: %+v", agents[1])
	}
}

func TestLoadAgentSessionsMissingDir(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "lonely.jsonl")
	if err := os.WriteFile(root, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agents, err := LoadAgentSessions(root, Options{})
	if err != nil || agents != nil {
		t.Fatalf("want (nil, nil), got (%v, %v)", agents, err)
	}
}

func TestAgentFileOptionParsesSidechainAsMainChain(t *testing.T) {
	s, err := Parse(strings.NewReader(agentRecord), Options{AgentFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("AgentFile must parse sidechain records as main chain; got %d turns", len(s.Turns))
	}
	// Without the option the sidechain record is dropped (no owner).
	s2, err := Parse(strings.NewReader(agentRecord), Options{IncludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Turns) != 0 {
		t.Fatalf("without AgentFile the record must not become a main turn; got %d", len(s2.Turns))
	}
}
