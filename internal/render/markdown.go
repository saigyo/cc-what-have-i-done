package render

import (
	"bytes"
	"html"
	"html/template"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// md renders markdown with GFM plus chroma-based syntax highlighting for fenced
// code blocks. Highlighting uses inline styles (WithClasses(false)) so the
// generated report stays fully self-contained — no external stylesheet.
var md = goldmark.New(goldmark.WithExtensions(
	extension.GFM,
	highlighting.NewHighlighting(
		highlighting.WithStyle("github"),
		highlighting.WithFormatOptions(
			chromahtml.WithClasses(false),
		),
	),
))

// Markdown renders CommonMark+GFM to HTML. goldmark runs with its default
// Unsafe=false setting, so any raw HTML embedded in the transcript text is
// stripped (replaced with an HTML comment) rather than passed through — the
// rendered Body is therefore safe to emit as template.HTML.
func Markdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML("<pre>" + html.EscapeString(src) + "</pre>")
	}
	return template.HTML(buf.String())
}

// DiffHTML renders a simple line-based diff of a Diff's old/new text.
func DiffHTML(d *model.Diff) template.HTML {
	var b strings.Builder
	b.WriteString(`<div class="diff">`)
	if d.OldText != "" {
		for _, line := range strings.Split(d.OldText, "\n") {
			b.WriteString(`<div class="diff-del">- `)
			b.WriteString(html.EscapeString(line))
			b.WriteString("</div>")
		}
	}
	if d.NewText != "" {
		for _, line := range strings.Split(d.NewText, "\n") {
			b.WriteString(`<div class="diff-add">+ `)
			b.WriteString(html.EscapeString(line))
			b.WriteString("</div>")
		}
	}
	b.WriteString("</div>")
	return template.HTML(b.String())
}
