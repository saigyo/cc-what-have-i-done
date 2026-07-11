package render

import (
	"bytes"
	"html"
	"html/template"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

var md = goldmark.New(goldmark.WithExtensions(extension.GFM))

// Markdown renders CommonMark+GFM to sanitized-enough HTML. Input is trusted
// transcript content (the user's own session), so we do not strip HTML, but we
// do escape code via goldmark's default renderer.
func Markdown(src string) template.HTML {
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML("<pre>" + html.EscapeString(src) + "</pre>")
	}
	return template.HTML(buf.String())
}

// Highlight renders a code string with chroma using inline styles (no external
// stylesheet needed). lang may be empty for auto-detection.
func Highlight(code, lang string) template.HTML {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("github")
	if style == nil {
		style = styles.Fallback
	}
	formatter := chromahtml.New(chromahtml.WithClasses(false), chromahtml.TabWidth(2))
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		return template.HTML("<pre>" + html.EscapeString(code) + "</pre>")
	}
	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return template.HTML("<pre>" + html.EscapeString(code) + "</pre>")
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
