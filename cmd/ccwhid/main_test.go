package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmdHasExpectedFlags(t *testing.T) {
	cmd := newRootCmd()
	for _, name := range []string{
		"session", "project", "latest", "out", "title",
		"include-subagents", "no-redact", "force", "open",
		"no-images", "license",
	} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected flag --%s to be registered", name)
		}
	}
}

func TestRootCmdHelpRuns(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--help returned error: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected help output, got none")
	}
}

func TestOptionsValidateMutuallyExclusiveNameFlags(t *testing.T) {
	if err := (&options{redactName: "Jane Doe", noRedactName: true}).validate(); err == nil {
		t.Error("expected error when --redact-name and --no-redact-name are combined")
	}
	for _, o := range []*options{
		{redactName: "Jane Doe"},
		{noRedactName: true},
		{},
	} {
		if err := o.validate(); err != nil {
			t.Errorf("validate(%+v) = %v, want nil", o, err)
		}
	}
}

func TestLicenseFlagPrintsNotices(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--license"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--license returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Third-party licenses for ccwhid") {
		t.Errorf("expected notices header in output, got: %.200s", out.String())
	}
	if !strings.Contains(out.String(), "github.com/spf13/cobra") {
		t.Errorf("expected full notices content (cobra section), got %d bytes", out.Len())
	}
}

func TestResolveVersion(t *testing.T) {
	cases := []struct {
		name, ldflags, mainVersion, want string
	}{
		{"ldflags wins", "0.6.0", "v9.9.9", "0.6.0"},
		{"module version, v stripped", "dev", "v0.6.0", "0.6.0"},
		{"devel falls back to dev", "dev", "(devel)", "dev"},
		{"empty falls back to dev", "dev", "", "dev"},
		{"pseudo-version passes through", "dev", "v0.6.1-0.20260731120000-abcdef123456", "0.6.1-0.20260731120000-abcdef123456"},
	}
	for _, c := range cases {
		if got := resolveVersion(c.ldflags, c.mainVersion); got != c.want {
			t.Errorf("%s: resolveVersion(%q, %q) = %q, want %q", c.name, c.ldflags, c.mainVersion, got, c.want)
		}
	}
}
