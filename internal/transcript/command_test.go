package transcript

import "testing"

func TestParseCommand(t *testing.T) {
	cases := []struct {
		name, text, want string
		ok               bool
	}{
		{"name only", "<command-name>/model</command-name>\n<command-message>model</command-message>\n<command-args></command-args>", "/model", true},
		{"name and args", "<command-name>/code-review</command-name><command-args>ultra 21</command-args>", "/code-review ultra 21", true},
		{"args whitespace trimmed", "<command-name> /clear </command-name><command-args>  </command-args>", "/clear", true},
		{"no closing tag", "<command-name>/model", "", false},
		{"no command markup", "just a prompt", "", false},
	}
	for _, c := range cases {
		got, ok := parseCommand(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: parseCommand = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestParseCommandOutput(t *testing.T) {
	cases := []struct {
		name, text, want string
		ok               bool
	}{
		{"stdout", "<local-command-stdout>Set model to \x1b[1mFable 5\x1b[22m</local-command-stdout>", "Set model to \x1b[1mFable 5\x1b[22m", true},
		{"stderr only", "<local-command-stderr>boom</local-command-stderr>", "boom", true},
		{"stdout and stderr", "<local-command-stdout>out</local-command-stdout><local-command-stderr>err</local-command-stderr>", "out\nerr", true},
		{"empty stdout", "<local-command-stdout></local-command-stdout>", "", true},
		{"no closing tag", "<local-command-stdout>oops", "", false},
		{"no output markup", "hello", "", false},
	}
	for _, c := range cases {
		got, ok := parseCommandOutput(c.text)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: parseCommandOutput = %q, %v; want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
