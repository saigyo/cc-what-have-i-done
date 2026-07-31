package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestLicenseFromGorootOfficialLayout(t *testing.T) {
	dir := t.TempDir()
	officialPath := dir + "/LICENSE"
	if err := os.WriteFile(officialPath, []byte("Official License\n"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	got, err := licenseFromGoroot(dir)
	if err != nil {
		t.Fatalf("licenseFromGoroot: %v", err)
	}
	if got != "Official License\n" {
		t.Errorf("got %q, want %q", got, "Official License\n")
	}
}

func TestLicenseFromGorootBrewLayout(t *testing.T) {
	tmpbase := t.TempDir()
	// Create tmpbase/goroot as the GOROOT, and tmpbase/LICENSE as the Homebrew layout
	gorootDir := tmpbase + "/goroot"
	if err := os.Mkdir(gorootDir, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	brewPath := tmpbase + "/LICENSE"
	if err := os.WriteFile(brewPath, []byte("Homebrew License\n"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	got, err := licenseFromGoroot(gorootDir)
	if err != nil {
		t.Fatalf("licenseFromGoroot: %v", err)
	}
	if got != "Homebrew License\n" {
		t.Errorf("got %q, want %q", got, "Homebrew License\n")
	}
}

func TestLicenseFromGorootNeitherExists(t *testing.T) {
	tmpDir := t.TempDir()
	goroot := filepath.Join(tmpDir, "goroot")
	if err := os.MkdirAll(goroot, 0o755); err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	_, err := licenseFromGoroot(goroot)
	if err == nil {
		t.Fatal("licenseFromGoroot: expected error, got nil")
	}
	errMsg := err.Error()
	officialPath := filepath.Join(goroot, "LICENSE")
	brewPath := filepath.Join(goroot, "..", "LICENSE")
	if !strings.Contains(errMsg, officialPath) || !strings.Contains(errMsg, brewPath) {
		t.Errorf("error message should mention both paths; got: %v", err)
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
