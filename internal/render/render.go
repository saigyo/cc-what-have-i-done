package render

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
	"github.com/saigyo/cc-what-have-i-done/internal/usage"
)

//go:embed assets/report.html.tmpl assets/styles.css assets/app.js
var assets embed.FS

// Options configures a render.
type Options struct {
	Title string
	Usage bool // render the token-usage & cost section
}

// Site renders the session into outDir as index.html + assets/.
func Site(s model.Session, outDir string, opts Options) error {
	title := opts.Title
	if title == "" {
		title = s.DisplayTitle()
	}

	tmplSrc, err := assets.ReadFile("assets/report.html.tmpl")
	if err != nil {
		return err
	}
	tmpl, err := template.New("report").Parse(string(tmplSrc))
	if err != nil {
		return err
	}

	data := buildViewModel(s, title, opts)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(outDir, "assets"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	for _, name := range []string{"styles.css", "app.js"} {
		b, err := assets.ReadFile("assets/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, "assets", name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// view models -------------------------------------------------------------

type viewData struct {
	Title     string
	Session   model.Session
	StartedAt string
	TurnCount int
	Prompts   []promptRef
	Turns     []turnView
	Usage     *usageView
}

type promptRef struct {
	Index   int
	Preview string
}

type turnView struct {
	Index      int
	Kind       string
	RoleLabel  string
	Status     string // agent-result status chip, e.g. "completed"
	SearchText string
	Body       template.HTML
	Badge      string // per-turn usage badge, e.g. "12k tok · ~$0.18"
}

type usageView struct {
	HasAny   bool
	Headline template.HTML // collapsed one-line summary; safe, built only from our own formatted numbers/words
	Models   []usageModel  // per-model rows, full token breakdown
	Total    usageModel    // grand-total row
	SubLine  string        // "of which subagents: …", empty when no linked agents
	Footnote string
}

// usageModel is one row of the per-model table (or the Total row). All fields
// are plain formatted strings, auto-escaped by html/template.
type usageModel struct {
	Model      string // model id, or "Total" for the summary row
	Input      string
	Output     string
	CacheRead  string
	CacheWrite string
	Cost       string // "$1.23" or "n/a"
}

func buildViewModel(s model.Session, title string, opts Options) viewData {
	d := viewData{
		Title:     title,
		Session:   s,
		TurnCount: len(s.Turns),
	}
	if !s.StartedAt.IsZero() {
		d.StartedAt = s.StartedAt.Format("2006-01-02 15:04")
	}

	var rep usage.Report
	if opts.Usage {
		rep = usage.Compute(s)
		d.Usage = buildUsageView(rep)
	}

	for i, t := range s.Turns {
		plain := turnPlainText(t)
		if t.Kind == model.TurnUser {
			d.Prompts = append(d.Prompts, promptRef{Index: i, Preview: preview(plain, 60)})
		}
		tv := turnView{
			Index:      i,
			Kind:       string(t.Kind),
			RoleLabel:  roleLabel(t),
			Status:     t.AgentStatus,
			SearchText: strings.ToLower(plain),
			Body:       renderTurnBody(t),
		}
		if opts.Usage && t.Usage != nil {
			tv.Badge = turnBadge(*t.Usage, rep.PerTurnCost[i])
		}
		d.Turns = append(d.Turns, tv)
	}
	return d
}

// turnBadge formats the per-turn usage badge: "12k tok" plus "· ~$0.18" if priced.
func turnBadge(u model.Usage, costUSD *float64) string {
	b := formatTokens(u.Input+u.Output) + " tok"
	if costUSD != nil {
		b += " · ~" + formatCost(*costUSD)
	}
	return b
}

// buildUsageView turns a usage.Report into the template-facing view model.
func buildUsageView(r usage.Report) *usageView {
	v := &usageView{HasAny: r.HasAnyUsage}
	if !r.HasAnyUsage {
		v.Headline = template.HTML("Usage · no token-usage data")
		return v
	}
	headline := "Usage · " + formatTokens(r.Total.InOut()) + " in+out"
	if r.TotalCostUSD != nil {
		headline += " · ~" + formatCost(*r.TotalCostUSD) + " (est.)"
	}
	// html.EscapeString (unlike html/template's auto-escaper) leaves "+" intact
	// while still escaping "<"/"&" etc.; headline is built solely from our own
	// formatted numbers and literal words, so this is safe to mark as raw HTML.
	v.Headline = template.HTML(html.EscapeString(headline))

	hasUnpriced := false
	for _, m := range r.ByModel {
		if m.Tokens == (usage.TokenCounts{}) {
			continue // drop all-zero rows (e.g. <synthetic> with no tokens)
		}
		v.Models = append(v.Models, modelRow(m.Model, m.Tokens, m.CostUSD))
		if m.CostUSD == nil {
			hasUnpriced = true
		}
	}
	v.Total = modelRow("Total", r.Total, r.TotalCostUSD)

	if r.AgentSessions > 0 {
		sub := "of which subagents: " + formatTokens(r.Subagents.InOut()) + " in+out"
		if r.SubagentsCost != nil {
			sub += " · ~" + formatCost(*r.SubagentsCost)
		}
		v.SubLine = fmt.Sprintf("%s (%d sessions)", sub, r.AgentSessions)
	}

	foot := "Estimated — Anthropic list prices as of " + r.PricesAsOf + "."
	if r.AgentSessions > 0 {
		foot += fmt.Sprintf(" Includes %d linked subagent session(s); server-tool fees are excluded.", r.AgentSessions)
	} else {
		foot += " Covers this transcript only; sub-agent sessions stored as separate files, and server-tool fees, are excluded."
	}
	if hasUnpriced {
		foot += " Estimated cost excludes unpriced models (shown as n/a); their tokens are still counted."
	}
	v.Footnote = foot
	return v
}

// modelRow formats one per-model (or Total) table row from a token bucket and
// optional cost.
func modelRow(name string, t usage.TokenCounts, costUSD *float64) usageModel {
	row := usageModel{
		Model:      name,
		Input:      formatTokens(t.Input),
		Output:     formatTokens(t.Output),
		CacheRead:  formatTokens(t.CacheRead),
		CacheWrite: formatTokens(t.CacheWrite5m + t.CacheWrite1h),
		Cost:       "n/a",
	}
	if costUSD != nil {
		row.Cost = formatCost(*costUSD)
	}
	return row
}

func roleLabel(t model.Turn) string {
	switch t.Kind {
	case model.TurnUser:
		return "You"
	case model.TurnAgentResult:
		return agentRoleLabel(t.AgentSummary)
	default:
		return "Claude"
	}
}

// agentRoleLabel derives `Agent · <name>` from a notification summary like
// `Agent "Implement Task 12" finished`; plain "Agent" when no quoted name.
func agentRoleLabel(summary string) string {
	if i := strings.Index(summary, `"`); i >= 0 {
		if j := strings.Index(summary[i+1:], `"`); j > 0 {
			return "Agent · " + summary[i+1:i+1+j]
		}
	}
	return "Agent"
}

// renderTurnBody renders all blocks of a turn to HTML.
func renderTurnBody(t model.Turn) template.HTML {
	var b strings.Builder
	for _, blk := range t.Blocks {
		switch blk.Type {
		case model.BlockText:
			b.WriteString(string(Markdown(blk.Text)))
		case model.BlockThinking:
			b.WriteString(`<details class="thinking"><summary>thinking</summary>`)
			b.WriteString(string(Markdown(blk.Text)))
			b.WriteString(`</details>`)
		case model.BlockToolUse:
			b.WriteString(renderTool(blk.Tool))
		}
	}
	return template.HTML(b.String())
}

func renderTool(tc *model.ToolCall) string {
	var b strings.Builder
	b.WriteString(`<details class="tool"><summary class="tool-head">`)
	b.WriteString(`<span class="tool-name">` + html.EscapeString(tc.Name) + `</span>`)
	if tc.Summary != "" {
		b.WriteString(`<span class="tool-summary">` + html.EscapeString(StripANSI(tc.Summary)) + `</span>`)
	}
	b.WriteString(`</summary><div class="tool-body">`)
	if tc.Diff != nil {
		b.WriteString(string(DiffHTML(tc.Diff)))
	} else if tc.InputJSON != "" {
		b.WriteString(`<pre class="tool-input">` + html.EscapeString(tc.InputJSON) + `</pre>`)
	}
	if tc.Result != nil && tc.Result.Content != "" {
		cls := "tool-result"
		if tc.Result.IsError {
			cls += " tool-result-error"
		}
		b.WriteString(`<pre class="` + cls + `">` + html.EscapeString(StripANSI(tc.Result.Content)) + `</pre>`)
	}
	for _, sub := range tc.Subagents {
		b.WriteString(`<details class="subagent"><summary>subagent: ` + html.EscapeString(sub.Description) + `</summary>`)
		for _, st := range sub.Turns {
			b.WriteString(`<article class="turn turn-` + string(st.Kind) + `"><div class="turn-role">` + roleLabel(st) + `</div><div class="turn-body">`)
			b.WriteString(string(renderTurnBody(st)))
			b.WriteString(`</div></article>`)
		}
		b.WriteString(`</details>`)
	}
	b.WriteString(`</div></details>`)
	return b.String()
}

func turnPlainText(t model.Turn) string {
	var parts []string
	for _, blk := range t.Blocks {
		switch blk.Type {
		case model.BlockText, model.BlockThinking:
			parts = append(parts, blk.Text)
		case model.BlockToolUse:
			parts = append(parts, blk.Tool.Name, blk.Tool.Summary)
		}
	}
	// Strip ANSI so previews and the search index carry clean text.
	return StripANSI(strings.Join(parts, " "))
}

func preview(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
