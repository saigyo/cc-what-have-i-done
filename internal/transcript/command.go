package transcript

import (
	"strings"

	"github.com/saigyo/cc-what-have-i-done/internal/model"
)

// Slash commands appear in transcripts as two consecutive user records:
// an input record tagged <command-name>/<command-args> and an output
// record tagged <local-command-stdout>/<local-command-stderr>. This file
// recognizes both so the parser can merge them into one command turn.

// tagContent extracts the content of the first <tag>…</tag> pair in text.
func tagContent(text, tag string) (string, bool) {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(text, open)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// parseCommand extracts a slash-command invocation: the <command-name>
// content plus, when non-empty, a space and the <command-args> content.
// The <command-message> echo is dropped.
func parseCommand(text string) (string, bool) {
	name, ok := tagContent(text, "command-name")
	if !ok {
		return "", false
	}
	inv := strings.TrimSpace(name)
	if args, ok := tagContent(text, "command-args"); ok {
		if args = strings.TrimSpace(args); args != "" {
			inv += " " + args
		}
	}
	return inv, true
}

// parseCommandOutput extracts local-command output: stdout first, then
// stderr, joined with a newline when both are present.
func parseCommandOutput(text string) (string, bool) {
	out, okOut := tagContent(text, "local-command-stdout")
	errText, okErr := tagContent(text, "local-command-stderr")
	if !okOut && !okErr {
		return "", false
	}
	out, errText = strings.TrimSpace(out), strings.TrimSpace(errText)
	switch {
	case out != "" && errText != "":
		return out + "\n" + errText, true
	case errText != "":
		return errText, true
	default:
		return out, true
	}
}

// userText concatenates a turn's text-block contents.
func userText(t *model.Turn) string {
	var b strings.Builder
	for _, blk := range t.Blocks {
		if blk.Type == model.BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// isOpenCommand reports whether t is a slash-command turn still awaiting
// its output record.
func isOpenCommand(t *model.Turn) bool {
	return t.Kind == model.TurnUser && len(t.Blocks) == 1 &&
		t.Blocks[0].Type == model.BlockCommand && t.Blocks[0].Command.Output == ""
}
