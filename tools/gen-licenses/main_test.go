package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIsNoticeName(t *testing.T) {
	for _, name := range []string{
		"LICENSE", "LICENSE.txt", "LICENSE.md", "license", "LICENCE",
		"COPYING", "COPYING.txt", "NOTICE", "NOTICE.md", "PATENTS",
	} {
		if !isNoticeName(name) {
			t.Errorf("isNoticeName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"README.md", "AUTHORS", "go.mod", "main.go",
		"license.go", "licenses.go", "notice.go",
	} {
		if isNoticeName(name) {
			t.Errorf("isNoticeName(%q) = true, want false", name)
		}
	}
}

func TestSectionSingleFile(t *testing.T) {
	m := module{path: "example.com/mod", version: "v1.2.3"}
	files := []noticeFile{{name: "LICENSE", text: "MIT text\n"}}
	got := section(sectionHeader(m, files), sectionBody(files))
	if !strings.Contains(got, "example.com/mod v1.2.3 (LICENSE)\n") {
		t.Errorf("single-file header should carry the filename, got:\n%s", got)
	}
	if !strings.Contains(got, "MIT text") {
		t.Errorf("section should contain the license text, got:\n%s", got)
	}
	if strings.Contains(got, "-- LICENSE --") {
		t.Errorf("single-file section must not use per-file separators, got:\n%s", got)
	}
	if !strings.Contains(got, strings.Repeat("=", 80)+"\n") {
		t.Errorf("section should use an 80-char separator line, got:\n%s", got)
	}
}

func TestSectionMultiFile(t *testing.T) {
	m := module{path: "example.com/mod", version: "v1.2.3"}
	files := []noticeFile{
		{name: "LICENSE", text: "BSD text\n"},
		{name: "PATENTS", text: "patent grant\n"},
	}
	got := section(sectionHeader(m, files), sectionBody(files))
	if !strings.Contains(got, "example.com/mod v1.2.3\n") ||
		strings.Contains(got, "(LICENSE)") {
		t.Errorf("multi-file header must not carry a filename, got:\n%s", got)
	}
	for _, marker := range []string{"-- LICENSE --", "-- PATENTS --", "BSD text", "patent grant"} {
		if !strings.Contains(got, marker) {
			t.Errorf("section should contain %q, got:\n%s", marker, got)
		}
	}
}

// TestGeneratedFileUpToDate is the freshness gate: it regenerates the
// notices from the current dependency graph and fails when the committed
// file is stale (e.g. after a dependency bump).
func TestGeneratedFileUpToDate(t *testing.T) {
	got, err := generate("../..")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want, err := os.ReadFile("../../THIRD_PARTY_LICENSES.txt")
	if err != nil {
		t.Fatalf("reading committed file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("THIRD_PARTY_LICENSES.txt is stale — regenerate with: go run ./tools/gen-licenses")
	}
}
