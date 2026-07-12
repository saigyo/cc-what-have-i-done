package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// agentMeta mirrors agent-<ID>.meta.json next to each agent transcript.
type agentMeta struct {
	AgentType   string `json:"agentType"`
	Description string `json:"description"`
	ToolUseID   string `json:"toolUseId"`
	SpawnDepth  int    `json:"spawnDepth"`
}

// LoadAgentSessions parses the linked subagent transcripts of a root session:
// <transcriptPath minus .jsonl>/subagents/agent-*.jsonl, sorted by file name.
// A missing subagents dir yields (nil, nil); an unparsable agent file is
// skipped with a warning on stderr. Nested agents (spawnDepth > 1) live in the
// same flat directory and are returned alongside depth-1 agents.
func LoadAgentSessions(transcriptPath string, opts Options) ([]model.AgentSession, error) {
	dir := filepath.Join(strings.TrimSuffix(transcriptPath, ".jsonl"), "subagents")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	opts.AgentFile = true
	var out []model.AgentSession
	for _, e := range entries { // ReadDir sorts by name
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "agent-"), ".jsonl")
		sess, err := ParseFile(filepath.Join(dir, name), opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping agent transcript %s: %v\n", name, err)
			continue
		}
		a := model.AgentSession{ID: id, Description: id, Session: sess}
		if b, err := os.ReadFile(filepath.Join(dir, "agent-"+id+".meta.json")); err == nil {
			var m agentMeta
			if json.Unmarshal(b, &m) == nil {
				if m.Description != "" {
					a.Description = m.Description
				}
				a.AgentType = m.AgentType
				a.ToolUseID = m.ToolUseID
				a.SpawnDepth = m.SpawnDepth
			}
		}
		out = append(out, a)
	}
	return out, nil
}
