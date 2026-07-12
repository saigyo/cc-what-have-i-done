package main

import (
	"bytes"
	"testing"
)

func TestRootCmdHasExpectedFlags(t *testing.T) {
	cmd := newRootCmd()
	for _, name := range []string{
		"session", "project", "latest", "out", "title",
		"include-subagents", "no-redact", "force", "open",
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
