// Package discovery finds Claude Code session transcripts under
// ~/.claude/projects and builds a lightweight index for listing and selection.
package discovery

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionInfo is a lightweight index entry for one transcript file. It is built
// by a cheap scan and does not parse the full timeline.
type SessionInfo struct {
	ID           string
	FilePath     string
	ProjectDir   string // encoded directory name
	ProjectPath  string // decoded absolute path
	Title        string
	FirstPrompt  string
	ModTime      time.Time
	MessageCount int
	HasSubagents bool
	// IsAgent reports that this transcript was produced by an SDK/agent
	// invocation (e.g. a Task subagent or code-review agent) rather than an
	// interactive human session. Such sessions get their own top-level
	// transcript files and are hidden from the default listing.
	IsAgent bool
}

// ProjectGroup groups a project's sessions.
type ProjectGroup struct {
	ProjectDir  string
	ProjectPath string
	Sessions    []SessionInfo
}

// EncodeProjectDir maps an absolute cwd to its ~/.claude/projects dir name.
func EncodeProjectDir(cwd string) string {
	return strings.ReplaceAll(cwd, "/", "-")
}

// DecodeProjectDir maps a projects dir name back to an absolute path.
//
// This is best-effort and lossy: the encoding used for ~/.claude/projects
// directory names replaces every "/" with "-", so a decode cannot distinguish
// an original path separator from a literal "-" inside a path segment (e.g.
// "cc-what-have-i-done" decodes to "cc/what/have/i/done"). Callers that need
// the true path should prefer the "cwd" field recorded in transcript records
// and only fall back to this function when no such record is available.
func DecodeProjectDir(name string) string {
	return strings.ReplaceAll(name, "-", "/")
}

// DefaultRoot returns ~/.claude/projects.
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// scanLine is the subset of fields needed to index a session cheaply.
type scanLine struct {
	Type         string          `json:"type"`
	AiTitle      string          `json:"aiTitle"`
	IsSidechain  bool            `json:"isSidechain"`
	Timestamp    string          `json:"timestamp"`
	Cwd          string          `json:"cwd"`
	Entrypoint   string          `json:"entrypoint"`
	PromptSource string          `json:"promptSource"`
	Message      json.RawMessage `json:"message"`
}

// indexFile reads a jsonl file and produces a SessionInfo without a full parse.
func indexFile(path, projectDir string) (SessionInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionInfo{}, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return SessionInfo{}, err
	}

	info := SessionInfo{
		ID:          strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		FilePath:    path,
		ProjectDir:  projectDir,
		ProjectPath: DecodeProjectDir(projectDir),
		ModTime:     fi.ModTime(),
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	cwdSet := false
	entrypointSet := false
	for sc.Scan() {
		var l scanLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue // tolerate malformed lines
		}
		switch l.Type {
		case "ai-title":
			if l.AiTitle != "" {
				info.Title = l.AiTitle
			}
		case "user", "assistant":
			info.MessageCount++
			if l.IsSidechain {
				info.HasSubagents = true
			}
			// Classify origin: the first record carrying an entrypoint decides
			// it (interactive "cli"/IDE vs. "sdk-*"), and any prompt sourced
			// from the SDK confirms an agent-spawned transcript.
			if !entrypointSet && l.Entrypoint != "" {
				info.IsAgent = strings.HasPrefix(l.Entrypoint, "sdk")
				entrypointSet = true
			}
			if l.PromptSource == "sdk" {
				info.IsAgent = true
			}
			if l.Type == "user" && info.FirstPrompt == "" {
				info.FirstPrompt = firstPromptText(l.Message)
			}
			if !cwdSet && l.Cwd != "" {
				info.ProjectPath = l.Cwd
				cwdSet = true
			}
		}
	}
	// A single pathologically long line (bufio.ErrTooLong) is tolerated like
	// the full parser does — index what we read so the session still lists.
	// Any other scan error means the file was truncated/unreadable, so signal
	// it and let Scan skip this entry rather than list a misleading index.
	if err := sc.Err(); err != nil && !errors.Is(err, bufio.ErrTooLong) {
		return SessionInfo{}, err
	}
	if info.Title == "" {
		info.Title = truncate(info.FirstPrompt, 60)
	}
	return info, nil
}

// firstPromptText extracts a plain-text preview from a user message whose
// content is either a JSON string or an array of blocks.
func firstPromptText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		return ""
	}
	var s string
	if json.Unmarshal(msg.Content, &s) == nil {
		return strings.TrimSpace(s)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(msg.Content, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" {
				return strings.TrimSpace(b.Text)
			}
		}
	}
	return ""
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Scan indexes every session under root (the projects directory), grouped by
// project. Groups are ordered by most-recent session first; sessions within a
// group are newest-first.
func Scan(root string) ([]ProjectGroup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var groups []ProjectGroup
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		if err != nil {
			return nil, err
		}
		var sessions []SessionInfo
		for _, fp := range files {
			si, err := indexFile(fp, e.Name())
			if err != nil {
				continue
			}
			sessions = append(sessions, si)
		}
		if len(sessions) == 0 {
			continue
		}
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].ModTime.After(sessions[j].ModTime)
		})
		groups = append(groups, ProjectGroup{
			ProjectDir:  e.Name(),
			ProjectPath: sessions[0].ProjectPath,
			Sessions:    sessions,
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Sessions[0].ModTime.After(groups[j].Sessions[0].ModTime)
	})
	return groups, nil
}

// RootSessions returns the group's interactive (non-agent) sessions, preserving
// order.
func (g ProjectGroup) RootSessions() []SessionInfo {
	out := make([]SessionInfo, 0, len(g.Sessions))
	for _, s := range g.Sessions {
		if !s.IsAgent {
			out = append(out, s)
		}
	}
	return out
}

// AgentCount returns how many of the group's sessions were agent-spawned.
func (g ProjectGroup) AgentCount() int {
	_, agent := g.Counts()
	return agent
}

// Counts returns the number of interactive (root) and agent-spawned sessions in
// a single pass, for callers that only need the tallies and not the slices.
func (g ProjectGroup) Counts() (root, agent int) {
	for _, s := range g.Sessions {
		if s.IsAgent {
			agent++
		} else {
			root++
		}
	}
	return root, agent
}

// FindProject resolves a project selector (full path, basename, or unambiguous
// substring of the path) to an index into groups. It errors if nothing matches
// or the selector is ambiguous.
func FindProject(groups []ProjectGroup, want string) (int, error) {
	// 1. Exact absolute-path match.
	for i, g := range groups {
		if g.ProjectPath == want {
			return i, nil
		}
	}
	// 2. Case-insensitive basename match.
	var matches []int
	for i, g := range groups {
		if strings.EqualFold(filepath.Base(g.ProjectPath), want) {
			matches = append(matches, i)
		}
	}
	// 3. Fall back to a case-insensitive substring of the full path.
	if len(matches) == 0 {
		lw := strings.ToLower(want)
		for i, g := range groups {
			if strings.Contains(strings.ToLower(g.ProjectPath), lw) {
				matches = append(matches, i)
			}
		}
	}
	switch len(matches) {
	case 0:
		return -1, fmt.Errorf("no project matching %q", want)
	case 1:
		return matches[0], nil
	default:
		return -1, fmt.Errorf("ambiguous project %q matches %d projects", want, len(matches))
	}
}

// DisplayLabel returns a human-friendly one-line label for a session.
func (s SessionInfo) DisplayLabel() string {
	if s.Title != "" {
		return s.Title
	}
	if s.FirstPrompt != "" {
		return s.FirstPrompt
	}
	return s.ID
}

// FindSession resolves a session id or unambiguous prefix across all projects.
func FindSession(root, idOrPrefix string) (SessionInfo, error) {
	groups, err := Scan(root)
	if err != nil {
		return SessionInfo{}, err
	}
	var matches []SessionInfo
	for _, g := range groups {
		for _, s := range g.Sessions {
			if s.ID == idOrPrefix {
				return s, nil // exact wins immediately
			}
			if strings.HasPrefix(s.ID, idOrPrefix) {
				matches = append(matches, s)
			}
		}
	}
	switch len(matches) {
	case 0:
		return SessionInfo{}, fmt.Errorf("no session matching %q", idOrPrefix)
	case 1:
		return matches[0], nil
	default:
		return SessionInfo{}, fmt.Errorf("ambiguous prefix %q matches %d sessions", idOrPrefix, len(matches))
	}
}
