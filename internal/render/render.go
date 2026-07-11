package render

import (
	"bytes"
	"embed"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

//go:embed assets/report.html.tmpl assets/styles.css assets/app.js
var assets embed.FS

// Options configures a render.
type Options struct {
	Title string
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

	data := buildViewModel(s, title)
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
}

type promptRef struct {
	Index   int
	Preview string
}

type turnView struct {
	Index      int
	Kind       string
	RoleLabel  string
	SearchText string
	Body       template.HTML
}

func buildViewModel(s model.Session, title string) viewData {
	d := viewData{
		Title:     title,
		Session:   s,
		TurnCount: len(s.Turns),
	}
	if !s.StartedAt.IsZero() {
		d.StartedAt = s.StartedAt.Format("2006-01-02 15:04")
	}
	for i, t := range s.Turns {
		plain := turnPlainText(t)
		if t.Kind == model.TurnUser {
			d.Prompts = append(d.Prompts, promptRef{Index: i, Preview: preview(plain, 60)})
		}
		d.Turns = append(d.Turns, turnView{
			Index:      i,
			Kind:       string(t.Kind),
			RoleLabel:  roleLabel(t.Kind),
			SearchText: strings.ToLower(plain),
			Body:       renderTurnBody(t),
		})
	}
	return d
}

func roleLabel(k model.TurnKind) string {
	if k == model.TurnUser {
		return "You"
	}
	return "Claude"
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
			b.WriteString(`<article class="turn turn-` + string(st.Kind) + `"><div class="turn-role">` + roleLabel(st.Kind) + `</div><div class="turn-body">`)
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
