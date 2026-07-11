// Package redact scrubs likely secrets and the user's home path from transcript
// text before it is rendered. It is best-effort defense-in-depth, not a
// guarantee of complete secret removal.
package redact

import (
	"regexp"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

type rule struct {
	kind string
	re   *regexp.Regexp
}

// Ordered: more specific patterns first.
var rules = []rule{
	{"aws-key", regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)},
	{"token", regexp.MustCompile(`\b(?:sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{50,}|xox[baprs]-[A-Za-z0-9-]{10,})\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"assignment", regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD))\s*[=:]\s*['"]?([^\s'"]{6,})`)},
}

// Redactor applies redaction rules and a home-directory rewrite.
type Redactor struct {
	homeDir string
}

func New(homeDir string) *Redactor { return &Redactor{homeDir: homeDir} }

// String redacts a single string.
func (r *Redactor) String(s string) string {
	for _, ru := range rules {
		if ru.kind == "assignment" {
			s = ru.re.ReplaceAllString(s, "$1=[REDACTED:assignment]")
			continue
		}
		s = ru.re.ReplaceAllString(s, "[REDACTED:"+ru.kind+"]")
	}
	if r.homeDir != "" {
		s = strings.ReplaceAll(s, r.homeDir, "~")
	}
	return s
}

// Session redacts every user-visible text field of a Session in place.
func Session(s *model.Session, homeDir string) {
	r := New(homeDir)
	for i := range s.Turns {
		redactTurn(r, &s.Turns[i])
	}
}

func redactTurn(r *Redactor, t *model.Turn) {
	for i := range t.Blocks {
		b := &t.Blocks[i]
		b.Text = r.String(b.Text)
		if b.Tool != nil {
			redactTool(r, b.Tool)
		}
	}
}

func redactTool(r *Redactor, tc *model.ToolCall) {
	tc.Summary = r.String(tc.Summary)
	tc.InputJSON = r.String(tc.InputJSON)
	if tc.Result != nil {
		tc.Result.Content = r.String(tc.Result.Content)
	}
	if tc.Diff != nil {
		tc.Diff.OldText = r.String(tc.Diff.OldText)
		tc.Diff.NewText = r.String(tc.Diff.NewText)
		tc.Diff.Path = r.String(tc.Diff.Path)
	}
	for i := range tc.Subagents {
		for j := range tc.Subagents[i].Turns {
			redactTurn(r, &tc.Subagents[i].Turns[j])
		}
	}
}
