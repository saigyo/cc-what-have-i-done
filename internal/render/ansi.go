package render

import (
	"regexp"
	"strings"
)

// ansiEscape matches ANSI/VT escape sequences: CSI (e.g. SGR colour codes like
// "\x1b[1m"), OSC sequences terminated by BEL or ST, and two-character Fe
// escapes. Terminal output leaks these into transcripts (e.g. a colourised
// `/model` command echo); the ESC byte is invisible in a browser but the
// trailing "[1m" shows as literal text, so we strip the whole sequence.
// The CSI ("\x1b[…") and OSC ("\x1b]…") alternatives are listed before the
// generic two-character Fe escape, whose class deliberately omits "[" and "]"
// so those introducers can't be swallowed as a short escape.
var ansiEscape = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\^_])`)

// StripANSI removes ANSI escape sequences from s. It fast-paths the common case
// of text containing no ESC byte at all.
func StripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	return ansiEscape.ReplaceAllString(s, "")
}
