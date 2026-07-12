package redact

import (
	"strings"
	"testing"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

func TestRedactPatterns(t *testing.T) {
	r := New(Config{HomeDir: "/Users/markus"})
	cases := []struct {
		in       string
		wantKind string
	}{
		{"key AKIAIOSFODNN7EXAMPLE here", "aws-key"},
		{"token sk-abcdefghijklmnopqrstuvwxy012345678", "token"},
		{"ghp_0123456789abcdefghijklmnopqrstuvwxyz", "token"},
		{"export API_KEY=supersecretvalue123", "assignment"},
	}
	for _, c := range cases {
		got := r.String(c.in)
		if !strings.Contains(got, "[REDACTED:"+c.wantKind+"]") {
			t.Errorf("String(%q) = %q, want kind %s", c.in, got, c.wantKind)
		}
	}
}

func TestRedactHomeDir(t *testing.T) {
	r := New(Config{HomeDir: "/Users/markus"})
	if got := r.String("/Users/markus/secret/file"); got != "~/secret/file" {
		t.Errorf("home rewrite = %q", got)
	}
}

func TestRedactSessionWalksBlocks(t *testing.T) {
	s := &model.Session{Turns: []model.Turn{{
		Kind: model.TurnAssistant,
		Blocks: []model.Block{
			{Type: model.BlockText, Text: "my key AKIAIOSFODNN7EXAMPLE"},
			{Type: model.BlockToolUse, Tool: &model.ToolCall{
				Name:    "Bash",
				Summary: "echo AKIAIOSFODNN7EXAMPLE",
				Result:  &model.ToolResult{Content: "AKIAIOSFODNN7EXAMPLE"},
			}},
		},
	}}}
	Session(s, Config{HomeDir: "/Users/markus"})
	if strings.Contains(s.Turns[0].Blocks[0].Text, "AKIA") {
		t.Error("block text not redacted")
	}
	tool := s.Turns[0].Blocks[1].Tool
	if strings.Contains(tool.Summary, "AKIA") || strings.Contains(tool.Result.Content, "AKIA") {
		t.Error("tool fields not redacted")
	}
}

func TestRedactSessionLevelFields(t *testing.T) {
	s := &model.Session{
		ProjectPath: "/Users/markus/IdeaProjects/app",
		Turns: []model.Turn{{
			Kind: model.TurnAssistant,
			Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
				Name:      "Task",
				Subagents: []model.Subagent{{Description: "run AKIAIOSFODNN7EXAMPLE"}},
			}}},
		}},
	}
	Session(s, Config{HomeDir: "/Users/markus"})
	if strings.Contains(s.ProjectPath, "/Users/markus") {
		t.Errorf("ProjectPath home not rewritten: %q", s.ProjectPath)
	}
	if strings.Contains(s.Turns[0].Blocks[0].Tool.Subagents[0].Description, "AKIA") {
		t.Error("subagent description not redacted")
	}
}

func TestRedactAgentPrompt(t *testing.T) {
	s := &model.Session{Turns: []model.Turn{{
		Kind: model.TurnAssistant,
		Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			Name:        "Agent",
			AgentPrompt: "read the brief at /Users/markus/IdeaProjects/app/brief.md and use AKIAIOSFODNN7EXAMPLE",
		}}},
	}}}
	Session(s, Config{HomeDir: "/Users/markus"})
	got := s.Turns[0].Blocks[0].Tool.AgentPrompt
	if strings.Contains(got, "/Users/markus") {
		t.Errorf("agent prompt home path not rewritten: %q", got)
	}
	if strings.Contains(got, "AKIA") {
		t.Errorf("agent prompt secret not redacted: %q", got)
	}
}

func TestRedactDashEncodedHomePath(t *testing.T) {
	// Claude Code encodes project dirs by replacing "/" with "-", so the $HOME
	// rewrite never sees them.
	r := New(Config{HomeDir: "/Users/markus"})
	got := r.String("~/.claude/projects/-Users-markus-IdeaProjects-cc-what-have-i-done/x.jsonl")
	if strings.Contains(got, "markus") {
		t.Errorf("dash-encoded username not scrubbed: %q", got)
	}
	if !strings.Contains(got, "-Users-[user]-IdeaProjects-") {
		t.Errorf("expected scrubbed encoded segment: %q", got)
	}
}

func TestRedactOtherUsersPath(t *testing.T) {
	// A foreign home path (different account) is scrubbed even though it isn't
	// our $HOME.
	r := New(Config{HomeDir: "/Users/markus"})
	for _, in := range []string{"/Users/alice/secret", `C:\Users\bob\file`, "/home/carol/x"} {
		got := r.String(in)
		for _, leak := range []string{"alice", "bob", "carol"} {
			if strings.Contains(got, leak) {
				t.Errorf("foreign account name leaked: %q -> %q", in, got)
			}
		}
	}
}

func TestRedactOwnerColumn(t *testing.T) {
	// `ls -l` owner column carries the username with no path context.
	r := New(Config{HomeDir: "/Users/markus"})
	got := r.String("drwx------@ 8 markus staff 256 baufinanzierung")
	if strings.Contains(got, "markus") {
		t.Errorf("bare username in owner column not scrubbed: %q", got)
	}
	if !strings.Contains(got, "[user] staff") {
		t.Errorf("expected scrubbed owner column: %q", got)
	}
}

func TestRedactKeepsHomeTilde(t *testing.T) {
	// The friendly ~ rewrite for our own home must survive.
	r := New(Config{HomeDir: "/Users/markus"})
	if got := r.String("/Users/markus/IdeaProjects/app"); got != "~/IdeaProjects/app" {
		t.Errorf("home tilde rewrite broken: %q", got)
	}
}

func TestAccountNameSkipsSystemAndShortNames(t *testing.T) {
	for _, h := range []string{"/root", "/home/ab", "/var/empty", ""} {
		if got := accountName(h); got != "" {
			t.Errorf("accountName(%q) = %q, want empty (system/short)", h, got)
		}
	}
	if got := accountName("/Users/markus"); got != "markus" {
		t.Errorf("accountName = %q, want markus", got)
	}
}

func TestRedactDoubledBackslashWindowsPath(t *testing.T) {
	r := New(Config{HomeDir: "/Users/markus"})
	if got := r.String(`C:\\Users\\bob\\file`); strings.Contains(got, "bob") {
		t.Errorf("doubled-backslash Windows path not scrubbed: %q", got)
	}
}

func TestRedactFullNameModulePathForms(t *testing.T) {
	r := New(Config{HomeDir: "/Users/markus", UserName: "Markus Ackermann"})
	cases := []struct{ in, notWant string }{
		{"import github.com/markusackermann/ccwhid/internal/model", "markusackermann"},
		{"see github.com/markus-ackermann/x", "markus-ackermann"},
		{"user markus.ackermann pushed", "markus.ackermann"},
		{"Signed-off-by: Markus Ackermann <x@y>", "Markus Ackermann"},
	}
	for _, c := range cases {
		got := r.String(c.in)
		if strings.Contains(got, c.notWant) {
			t.Errorf("String(%q) = %q, still contains %q", c.in, got, c.notWant)
		}
		if !strings.Contains(got, userPlaceholder) {
			t.Errorf("String(%q) = %q, expected %q", c.in, got, userPlaceholder)
		}
	}
	// The canonical module-path leak collapses to the placeholder, keeping the rest.
	if got := r.String("github.com/markusackermann/ccwhid"); got != "github.com/[user]/ccwhid" {
		t.Errorf("module path scrub = %q, want github.com/[user]/ccwhid", got)
	}
}

func TestRedactFullNameLeavesLegitimateRefsIntact(t *testing.T) {
	r := New(Config{HomeDir: "/Users/markus", UserName: "Markus Ackermann"})
	// The real module path must be untouched — only the known name is scrubbed.
	if got := r.String("github.com/saigyo/cc-what-have-i-done"); got != "github.com/saigyo/cc-what-have-i-done" {
		t.Errorf("legitimate ref altered: %q", got)
	}
}

func TestRedactSingleTokenNameNotScrubbed(t *testing.T) {
	// A one-word display name is too risky to scrub broadly (could be a common
	// word) and is left to the account-name rule; nameForms returns nothing.
	if forms := nameForms("Apple"); forms != nil {
		t.Errorf("single-token name should yield no forms, got %v", forms)
	}
	r := New(Config{HomeDir: "/Users/markus", UserName: "Apple"})
	if got := r.String("the Apple falls"); got != "the Apple falls" {
		t.Errorf("single-token name over-redacted: %q", got)
	}
}

func TestRedactNoUserNameKeepsNameForms(t *testing.T) {
	// Without a configured name, compound-name forms are not scrubbed.
	r := New(Config{HomeDir: "/Users/markus"})
	if got := r.String("github.com/markusackermann/ccwhid"); !strings.Contains(got, "markusackermann") {
		t.Errorf("no UserName should leave markusackermann intact: %q", got)
	}
}

func TestRedactScrubsAskUserQuestionFields(t *testing.T) {
	// AskUserQuestion cards render from the structured Questions, which must be
	// scrubbed just like every other user-visible field.
	s := &model.Session{Turns: []model.Turn{{
		Kind: model.TurnAssistant,
		Blocks: []model.Block{{Type: model.BlockToolUse, Tool: &model.ToolCall{
			Name: "AskUserQuestion",
			Questions: []model.Question{{
				Header: "Path for markus",
				Prompt: "Use /Users/markus/secret?",
				Options: []model.QuestionOption{{
					Label:       "Yes, markusackermann",
					Description: "keep ~/notes",
					Preview:     "cat /Users/markus/.env",
				}},
			}},
		}}},
	}}}
	Session(s, Config{HomeDir: "/Users/markus", UserName: "Markus Ackermann"})
	q := s.Turns[0].Blocks[0].Tool.Questions[0]
	blob := q.Header + "\n" + q.Prompt + "\n" + q.Options[0].Label + "\n" + q.Options[0].Description + "\n" + q.Options[0].Preview
	for _, leak := range []string{"markusackermann", "/Users/markus"} {
		if strings.Contains(blob, leak) {
			t.Errorf("AskUserQuestion field still leaks %q: %q", leak, blob)
		}
	}
}

func TestRedactFullNameUnicodeBoundaries(t *testing.T) {
	// A name ending in an accented letter would slip past RE2's ASCII-only \b;
	// the Unicode delimiter capture must still scrub it and keep the delimiters.
	r := New(Config{HomeDir: "/home/ana", UserName: "Ana Peñá"})
	cases := []struct{ in, want string }{
		{"repo github.com/anapeñá/x", "repo github.com/[user]/x"},
		{"by Ana Peñá.", "by [user]."},
		{"user ana-peñá pushed", "user [user] pushed"},
	}
	for _, c := range cases {
		if got := r.String(c.in); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
