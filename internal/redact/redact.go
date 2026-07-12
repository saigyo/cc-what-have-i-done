// Package redact scrubs likely secrets and the user's home path from transcript
// text before it is rendered. It is best-effort defense-in-depth, not a
// guarantee of complete secret removal.
package redact

import (
	"regexp"
	"sort"
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

// userPathRe matches a home-directory segment in slash, backslash, or Claude
// Code's dash-encoded form — "/Users/alice", "C:\Users\alice", or
// "-Users-alice-IdeaProjects" — capturing the account name that follows
// "Users"/"home". It scrubs account names even in paths outside our own home
// (other users, dash-encoded project dirs) that the $HOME rewrite never sees.
// The separator runs use "+" so JSON-escaped Windows paths (`\\Users\\bob`)
// match as well as single-separator forms.
var userPathRe = regexp.MustCompile(`(?i)([/\\-]+)(Users|home)([/\\-]+)([^/\\-]+)`)

// userPlaceholder replaces a scrubbed account name.
const userPlaceholder = "[user]"

// Config holds the known identity a Redactor scrubs. Both fields are optional;
// an empty field disables the corresponding rule.
type Config struct {
	HomeDir  string // the user's home directory, rewritten to ~ and mined for the account name
	UserName string // the user's display name, e.g. "Markus Ackermann"; its path/module forms are scrubbed
}

// Redactor applies redaction rules, a home-directory rewrite, and account-name
// and display-name heuristics.
type Redactor struct {
	homeDir string
	userRe  *regexp.Regexp // standalone mentions of our own account name; nil when unknown/unsafe
	nameRe  *regexp.Regexp // path/module/verbatim forms of the display name; nil when unknown/unsafe
}

func New(cfg Config) *Redactor {
	r := &Redactor{homeDir: cfg.HomeDir}
	if name := accountName(cfg.HomeDir); name != "" {
		r.userRe = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(name) + `\b`)
	}
	if forms := nameForms(cfg.UserName); len(forms) > 0 {
		// Match longer, more-specific forms first so "markusackermann" wins over
		// any shorter overlap.
		sort.Slice(forms, func(i, j int) bool { return len(forms[i]) > len(forms[j]) })
		alts := make([]string, len(forms))
		for i, f := range forms {
			alts[i] = regexp.QuoteMeta(f)
		}
		// Delimit with Unicode letter/number classes rather than \b: RE2's \b is
		// ASCII-only, so it mishandles accented display names (e.g. a form ending
		// in "á"). Capture the surrounding delimiters and re-emit them, since RE2
		// has no lookarounds.
		r.nameRe = regexp.MustCompile(`(?i)(^|[^\pL\pN])(?:` + strings.Join(alts, "|") + `)($|[^\pL\pN])`)
	}
	return r
}

// nameForms derives the literal strings to scrub from a display name: the
// verbatim name plus its concatenated and separator-joined lowercase forms
// (module paths, usernames). It returns nil for single-token names — those are
// already covered by the account-name rule, and scrubbing a lone word (which
// could be a common noun or company name) risks mangling unrelated text.
func nameForms(full string) []string {
	parts := strings.Fields(full)
	if len(parts) < 2 {
		return nil
	}
	lower := make([]string, len(parts))
	for i, p := range parts {
		lower[i] = strings.ToLower(p)
	}
	candidates := []string{
		strings.Join(parts, " "), // "Markus Ackermann"
		strings.Join(lower, ""),  // markusackermann
		strings.Join(lower, "-"), // markus-ackermann
		strings.Join(lower, "."), // markus.ackermann
		strings.Join(lower, "_"), // markus_ackermann
	}
	var forms []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if len(c) < 4 || seen[c] {
			continue
		}
		seen[c] = true
		forms = append(forms, c)
	}
	return forms
}

// accountName extracts the login name from a home-directory path. It returns ""
// for very short names or common system accounts, where scrubbing the bare word
// would mangle unrelated output (e.g. "root" in log lines).
func accountName(homeDir string) string {
	name := strings.TrimRight(homeDir, `/\`)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if len(name) < 3 {
		return ""
	}
	switch strings.ToLower(name) {
	case "root", "home", "user", "users", "admin", "ubuntu", "shared", "guest", "empty", "nobody", "daemon":
		return ""
	}
	return name
}

// String redacts a single string.
func (r *Redactor) String(s string) string {
	for _, ru := range rules {
		if ru.kind == "assignment" {
			s = ru.re.ReplaceAllString(s, "$1=[REDACTED:assignment]")
			continue
		}
		s = ru.re.ReplaceAllString(s, "[REDACTED:"+ru.kind+"]")
	}
	// Rewrite our own home directory to ~ first, so the common case stays
	// readable ("~/foo" rather than "/Users/[user]/foo").
	if r.homeDir != "" {
		s = strings.ReplaceAll(s, r.homeDir, "~")
	}
	// Scrub the display name's path/module/verbatim forms (e.g. the account name
	// in a Go module path, "github.com/markusackermann/…"), which the account-name
	// rule below deliberately misses because of its word boundary.
	if r.nameRe != nil {
		s = r.nameRe.ReplaceAllString(s, "${1}"+userPlaceholder+"${2}")
	}
	// Scrub account names in any remaining home-style path: dash-encoded project
	// dirs, other users' home paths, Windows paths.
	s = userPathRe.ReplaceAllString(s, "$1$2$3"+userPlaceholder)
	// Scrub standalone mentions of our own account name that carry no path
	// context — e.g. the owner column of `ls -l` output.
	if r.userRe != nil {
		s = r.userRe.ReplaceAllString(s, userPlaceholder)
	}
	return s
}

// Session redacts every user-visible text field of a Session in place.
func Session(s *model.Session, cfg Config) {
	r := New(cfg)
	// Session-level fields are rendered in the report header, so they must be
	// scrubbed too (ProjectPath in particular carries the home path/username).
	s.ProjectPath = r.String(s.ProjectPath)
	s.Title = r.String(s.Title)
	s.GitBranch = r.String(s.GitBranch)
	for i := range s.Turns {
		redactTurn(r, &s.Turns[i])
	}
	for i := range s.Agents {
		s.Agents[i].Description = r.String(s.Agents[i].Description)
		Session(&s.Agents[i].Session, cfg)
	}
}

func redactTurn(r *Redactor, t *model.Turn) {
	t.AgentSummary = r.String(t.AgentSummary)
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
	tc.AgentPrompt = r.String(tc.AgentPrompt)
	// AskUserQuestion cards render from the structured Questions, not InputJSON,
	// so every field they surface must be scrubbed too.
	for i := range tc.Questions {
		q := &tc.Questions[i]
		q.Header = r.String(q.Header)
		q.Prompt = r.String(q.Prompt)
		for j := range q.Options {
			o := &q.Options[j]
			o.Label = r.String(o.Label)
			o.Description = r.String(o.Description)
			o.Preview = r.String(o.Preview)
		}
	}
	if tc.Result != nil {
		tc.Result.Content = r.String(tc.Result.Content)
	}
	if tc.Diff != nil {
		tc.Diff.OldText = r.String(tc.Diff.OldText)
		tc.Diff.NewText = r.String(tc.Diff.NewText)
		tc.Diff.Path = r.String(tc.Diff.Path)
	}
	for i := range tc.Subagents {
		tc.Subagents[i].Description = r.String(tc.Subagents[i].Description)
		for j := range tc.Subagents[i].Turns {
			redactTurn(r, &tc.Subagents[i].Turns[j])
		}
	}
}
