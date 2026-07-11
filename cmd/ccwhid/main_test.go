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
