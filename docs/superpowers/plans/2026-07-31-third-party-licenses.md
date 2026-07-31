# Third-Party Licenses Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship complete third-party license notices with every ccwhid binary: a generated `THIRD_PARTY_LICENSES.txt` (all linked modules across the release GOOS matrix + Go stdlib), embedded and printed by `--license`, packed into release archives, guarded by a freshness test and a lichen license check in CI, plus a curated direct-deps table in the README.

**Architecture:** A stdlib-only generator (`tools/gen-licenses`) unions `go list -deps ./cmd/ccwhid` module sets across GOOS=linux/darwin/windows, reads each module's notice files from the module cache, and writes one deterministic text file at the repo root. A new root package embeds that file (`go:embed` cannot reach upward, so the embed lives next to the file); the cobra root command prints it verbatim on `--license`. CI regenerates the file in a test (staleness gate) and runs lichen against cross-compiled binaries (license-policy gate).

**Tech Stack:** Go 1.26 stdlib only (os/exec, embed, flag); uw-labs/lichen v0.1.7 via `go run` in CI (never enters go.mod); GitHub Actions; GoReleaser.

## Global Constraints

- **No new go.mod dependencies.** The generator is stdlib-only; lichen runs via `go run github.com/uw-labs/lichen@v0.1.7` (pinned) and must not be added to go.mod.
- **Coverage:** union of linked modules for GOOS `linux`, `darwin`, `windows` from `go list -deps ./cmd/ccwhid`; main module excluded; plus a Go-stdlib section (BSD-3-Clause) read from `$(go env GOROOT)/LICENSE`.
- **Notice-file matcher:** module-root files whose names start (case-insensitively) with `LICENSE`, `LICENCE`, `COPYING`, `NOTICE`, or `PATENTS`; names ending in `.go` are excluded.
- **Override:** `github.com/mattn/go-localereader` has no license file in v0.0.1; its section quotes the upstream README's MIT declaration. Any other module without notice files is a fatal generator error.
- **Determinism:** sections sorted by module path, stdlib section first, no timestamps, 80-char `=` separator lines, file ends with exactly one trailing newline.
- **Flag help text, verbatim:** `print third-party license information and exit`.
- **`--license` short-circuits** at the top of `run` before validation and session resolution; other flags are ignored, not an error.
- All code gofmt-clean, `go vet ./...` clean, tests pass with `-race`. Tests use plain `testing`, no new libraries.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

## File Structure

- `tools/gen-licenses/main.go` — generator (package main, also exposes `generate` for tests)
- `tools/gen-licenses/main_test.go` — unit tests + freshness test
- `THIRD_PARTY_LICENSES.txt` — generated, committed at repo root
- `third_party_licenses.go` — new root package `ccwhid`, embeds the txt
- `cmd/ccwhid/main.go` — `--license` flag + short-circuit
- `cmd/ccwhid/main_test.go` — flag-list + `--license` output tests
- `lichen.yaml` — license allowlist + documented exceptions
- `.github/workflows/ci.yml` — lichen step
- `.goreleaser.yaml` — archive the txt
- `README.md` — Third-party licenses section + Flags row

---

### Task 1: License-notices generator + generated file

**Files:**
- Create: `tools/gen-licenses/main.go`
- Create: `tools/gen-licenses/main_test.go`
- Create: `THIRD_PARTY_LICENSES.txt` (generated output, committed)

**Interfaces:**
- Consumes: nothing from other tasks. Runs `go list` / `go env` via os/exec; reads the local module cache (already populated — do NOT run `go mod download`, it is already done).
- Produces: `THIRD_PARTY_LICENSES.txt` at the repo root (Task 2 embeds it; Task 3 archives it); `go run ./tools/gen-licenses` as the regeneration command; unexported helpers `generate(root string) ([]byte, error)`, `isNoticeName(name string) bool`, `section(header, body string) string`, `sectionHeader(m module, files []noticeFile) string`, `sectionBody(files []noticeFile) string`.

- [ ] **Step 1: Write the failing unit tests**

Create `tools/gen-licenses/main_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./tools/gen-licenses/`
Expected: FAIL to build — `undefined: isNoticeName`, `undefined: module`, etc.

- [ ] **Step 3: Implement the generator**

Create `tools/gen-licenses/main.go`:

```go
// Command gen-licenses writes THIRD_PARTY_LICENSES.txt: the license and
// notice texts of every Go module linked into ccwhid release binaries
// (union across linux, darwin, and windows builds), plus the Go standard
// library. Run it from the repo root:
//
//	go run ./tools/gen-licenses
//
// CI fails when the committed file is stale (TestGeneratedFileUpToDate)
// and separately checks the licenses themselves with lichen.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// releaseGOOS is the GOOS matrix of release builds (see .goreleaser.yaml).
// The linked module set is platform-dependent (e.g. Windows-only console
// helpers), so the notices cover the union.
var releaseGOOS = []string{"linux", "darwin", "windows"}

// overrides supplies a notice text for modules that ship none. Any module
// without notice files and without an entry here is a fatal error.
var overrides = map[string]string{
	"github.com/mattn/go-localereader": `The tagged release of this module contains no license file.
Its upstream README (https://github.com/mattn/go-localereader) declares:

    License: MIT
    Author: Yasuhiro Matsumoto (a.k.a. mattn)
`,
}

const preamble = `Third-party licenses for ccwhid (github.com/saigyo/cc-what-have-i-done)

This file covers every Go module linked into ccwhid release binaries
(union across linux, darwin, and windows builds) plus the Go standard
library. Generated by tools/gen-licenses; verified in CI.

`

type module struct {
	path, version, dir string
}

type noticeFile struct {
	name, text string
}

func main() {
	out := flag.String("o", "THIRD_PARTY_LICENSES.txt", "output file path")
	flag.Parse()
	data, err := generate(".")
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-licenses:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gen-licenses:", err)
		os.Exit(1)
	}
}

// generate builds the complete notices file for the module rooted at
// root (the repo checkout).
func generate(root string) ([]byte, error) {
	mods := map[string]module{}
	for _, goos := range releaseGOOS {
		list, err := listModules(root, goos)
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			mods[m.path] = m
		}
	}

	var b bytes.Buffer
	b.WriteString(preamble)

	stdText, err := gorootLicense(root)
	if err != nil {
		return nil, err
	}
	b.WriteString(section("Go standard library and runtime (BSD-3-Clause)", stdText))

	paths := make([]string, 0, len(mods))
	for p := range mods {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		m := mods[p]
		files, err := noticeFiles(m.dir)
		if err != nil {
			return nil, fmt.Errorf("module %s: %w", p, err)
		}
		if len(files) == 0 {
			txt, ok := overrides[p]
			if !ok {
				return nil, fmt.Errorf("module %s has no notice files (LICENSE/COPYING/NOTICE) and no override — investigate before shipping", p)
			}
			b.WriteString(section(p+" "+m.version+" (no license file — see note)", txt))
			continue
		}
		b.WriteString(section(sectionHeader(m, files), sectionBody(files)))
	}
	return append(bytes.TrimRight(b.Bytes(), "\n"), '\n'), nil
}

// listModules returns the modules providing packages to ./cmd/ccwhid for
// one GOOS, excluding the main module.
func listModules(root, goos string) ([]module, error) {
	cmd := exec.Command("go", "list", "-deps",
		"-f", "{{if and .Module (not .Module.Main)}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}",
		"./cmd/ccwhid")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS="+goos)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("go list (GOOS=%s): %v\n%s", goos, err, ee.Stderr)
		}
		return nil, fmt.Errorf("go list (GOOS=%s): %w", goos, err)
	}
	seen := map[string]bool{}
	var mods []module
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("go list (GOOS=%s): unexpected line %q", goos, line)
		}
		if parts[2] == "" {
			return nil, fmt.Errorf("module %s is not in the local module cache — run 'go mod download' first", parts[0])
		}
		mods = append(mods, module{path: parts[0], version: parts[1], dir: parts[2]})
	}
	return mods, nil
}

// gorootLicense reads the Go standard library's license text.
func gorootLicense(root string) (string, error) {
	cmd := exec.Command("go", "env", "GOROOT")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOROOT: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "LICENSE"))
	if err != nil {
		return "", fmt.Errorf("reading Go stdlib license: %w", err)
	}
	return string(data), nil
}

// noticeFiles returns the notice files at a module root, sorted by name
// (os.ReadDir returns sorted entries).
func noticeFiles(dir string) ([]noticeFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []noticeFile
	for _, e := range entries {
		if e.IsDir() || !isNoticeName(e.Name()) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, noticeFile{name: e.Name(), text: string(data)})
	}
	return out, nil
}

// isNoticeName reports whether a file name looks like a license or
// notice document. Source files are excluded (a module root may contain
// license.go).
func isNoticeName(name string) bool {
	u := strings.ToUpper(name)
	if strings.HasSuffix(u, ".GO") {
		return false
	}
	for _, prefix := range []string{"LICENSE", "LICENCE", "COPYING", "NOTICE", "PATENTS"} {
		if strings.HasPrefix(u, prefix) {
			return true
		}
	}
	return false
}

// sectionHeader is "path version", plus "(filename)" when the module has
// exactly one notice file.
func sectionHeader(m module, files []noticeFile) string {
	h := m.path + " " + m.version
	if len(files) == 1 {
		h += " (" + files[0].name + ")"
	}
	return h
}

// sectionBody is the notice text; multiple files are concatenated, each
// preceded by a "-- name --" line.
func sectionBody(files []noticeFile) string {
	if len(files) == 1 {
		return files[0].text
	}
	parts := make([]string, 0, len(files))
	for _, f := range files {
		parts = append(parts, "-- "+f.name+" --\n\n"+strings.TrimRight(f.text, "\n")+"\n")
	}
	return strings.Join(parts, "\n")
}

// section renders one block: separator, header, separator, blank line,
// body, blank line.
func section(header, body string) string {
	sep := strings.Repeat("=", 80)
	return sep + "\n" + header + "\n" + sep + "\n\n" + strings.TrimRight(body, "\n") + "\n\n"
}
```

- [ ] **Step 4: Run the unit tests**

Run: `go test ./tools/gen-licenses/ -run 'TestIsNoticeName|TestSection'`
Expected: PASS (TestGeneratedFileUpToDate would still fail — the file doesn't exist yet).

- [ ] **Step 5: Generate the notices file**

Run from the repo root: `go run ./tools/gen-licenses`
Expected: exit 0, `THIRD_PARTY_LICENSES.txt` created.

Sanity-check the output:

Run: `head -12 THIRD_PARTY_LICENSES.txt && grep -c '^====' THIRD_PARTY_LICENSES.txt && grep -n 'go-localereader\|PATENTS\|standard library' THIRD_PARTY_LICENSES.txt | head`
Expected: the preamble text; 54 separator lines (27 sections × 2: stdlib + 26 modules); a `go-localereader … (no license file — see note)` header; `-- PATENTS --` markers under the two golang.org/x modules; the stdlib section header.

- [ ] **Step 6: Run all package tests (freshness gate now green)**

Run: `go test ./tools/gen-licenses/`
Expected: PASS, including TestGeneratedFileUpToDate.

- [ ] **Step 7: Verify formatting and vet**

Run: `gofmt -l tools/ && go vet ./tools/...`
Expected: no output from gofmt, vet clean.

- [ ] **Step 8: Commit**

```bash
git add tools/gen-licenses/main.go tools/gen-licenses/main_test.go THIRD_PARTY_LICENSES.txt
git commit -m "feat(licenses): generator for THIRD_PARTY_LICENSES.txt with freshness test"
```

---

### Task 2: Root embed package + `--license` flag

**Files:**
- Create: `third_party_licenses.go` (repo root)
- Modify: `cmd/ccwhid/main.go` (options struct ~line 18-32, flag block ~line 56-69, top of `run` ~line 73)
- Test: `cmd/ccwhid/main_test.go`

**Interfaces:**
- Consumes: `THIRD_PARTY_LICENSES.txt` at the repo root (committed in Task 1; starts with the line `Third-party licenses for ccwhid (github.com/saigyo/cc-what-have-i-done)`).
- Produces: root package `ccwhid` (import path `github.com/saigyo/cc-what-have-i-done`) exporting `ThirdPartyLicenses string`; CLI flag `--license`.

- [ ] **Step 1: Write the failing tests**

In `cmd/ccwhid/main_test.go`, add `"license"` to the flag list in `TestRootCmdHasExpectedFlags` (after `"no-images"`):

```go
	for _, name := range []string{
		"session", "project", "latest", "out", "title",
		"include-subagents", "no-redact", "force", "open",
		"no-images", "license",
	} {
```

And append this test (the file already imports `bytes` and `testing`; add `strings` to the import block):

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/ccwhid/ -run 'TestRootCmdHasExpectedFlags|TestLicenseFlagPrintsNotices'`
Expected: FAIL — flag `--license` not registered; the second test fails with `unknown flag: --license`.

- [ ] **Step 3: Create the root embed package**

Create `third_party_licenses.go` at the repo root:

```go
// Package ccwhid exposes repo-root assets that ship inside the binary.
package ccwhid

import _ "embed"

// ThirdPartyLicenses is the complete third-party notices file, generated
// by tools/gen-licenses and verified fresh in CI.
//
//go:embed THIRD_PARTY_LICENSES.txt
var ThirdPartyLicenses string
```

- [ ] **Step 4: Wire the flag**

In `cmd/ccwhid/main.go`:

1. Add the import (aliased — the directory name is not a valid identifier):

```go
	ccwhid "github.com/saigyo/cc-what-have-i-done"
```

2. Add to the `options` struct, after `noImages bool`:

```go
	license          bool
```

3. Register the flag in `newRootCmd`, after the `no-images` line:

```go
	f.BoolVar(&opts.license, "license", false, "print third-party license information and exit")
```

4. Short-circuit at the very top of `run`, before `opts.validate()`:

```go
	if opts.license {
		fmt.Fprint(cmd.OutOrStdout(), ccwhid.ThirdPartyLicenses)
		return nil
	}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./cmd/ccwhid/`
Expected: PASS (all, including existing tests).

- [ ] **Step 6: Verify the built binary end-to-end**

Run: `go build -o /tmp/ccwhid-license-check ./cmd/ccwhid && /tmp/ccwhid-license-check --license | head -3 && /tmp/ccwhid-license-check --license | wc -l && rm /tmp/ccwhid-license-check`
Expected: the preamble's first lines; a line count in the thousands (full notice texts).

- [ ] **Step 7: Verify formatting, vet, full suite**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 8: Commit**

```bash
git add third_party_licenses.go cmd/ccwhid/main.go cmd/ccwhid/main_test.go
git commit -m "feat(cli): --license flag prints embedded third-party notices"
```

---

### Task 3: lichen CI gate, release archives, README

**Files:**
- Create: `lichen.yaml`
- Modify: `.github/workflows/ci.yml` (add step after "Build")
- Modify: `.goreleaser.yaml` (archives files list, ~line 10 of the archives block)
- Modify: `README.md` (Flags table ~line 90; new section above `## License` ~line 148)

**Interfaces:**
- Consumes: `THIRD_PARTY_LICENSES.txt` (Task 1), the `--license` flag (Task 2, for README copy only).
- Produces: CI license gate; archives containing the notices file.

- [ ] **Step 1: Create `lichen.yaml`**

This exact config was verified passing against cross-compiled linux+windows binaries:

```yaml
allow:
  - "MIT"
  - "Apache-2.0"
  - "BSD-3-Clause"
  - "BSD-2-Clause"
  - "ISC"

exceptions:
  licenseNotPermitted:
    # chroma's COPYING includes the OFL-1.1 text for the Liberation Mono
    # font embedded by its SVG formatter; ccwhid does not import
    # formatters/svg, so the font is not part of our binaries.
    - path: "github.com/alecthomas/chroma/v2"
      licenses: ["OFL-1.1"]
  unresolvableLicense:
    # v0.0.1 ships no LICENSE file; upstream README declares MIT
    # (see THIRD_PARTY_LICENSES.txt entry).
    - path: "github.com/mattn/go-localereader"
```

- [ ] **Step 2: Verify lichen passes locally**

Run:

```bash
mkdir -p /tmp/lichen-bin
GOOS=linux   go build -o /tmp/lichen-bin/ccwhid-linux   ./cmd/ccwhid
GOOS=windows go build -o /tmp/lichen-bin/ccwhid-windows ./cmd/ccwhid
go run github.com/uw-labs/lichen@v0.1.7 --config=lichen.yaml \
  /tmp/lichen-bin/ccwhid-linux /tmp/lichen-bin/ccwhid-windows
rm -rf /tmp/lichen-bin
```

Expected: every module line ends `(allowed)`, exit 0. (chroma shows `MIT, OFL-1.1 (allowed)`; go-localereader shows an empty license `(allowed)` — both via the documented exceptions.)

- [ ] **Step 3: Add the CI step**

In `.github/workflows/ci.yml`, after the `- name: Build` step and before `- name: Test (race + coverage)`, insert:

```yaml
      - name: License check (lichen)
        run: |
          mkdir -p /tmp/lichen-bin
          GOOS=linux   go build -o /tmp/lichen-bin/ccwhid-linux   ./cmd/ccwhid
          GOOS=windows go build -o /tmp/lichen-bin/ccwhid-windows ./cmd/ccwhid
          go run github.com/uw-labs/lichen@v0.1.7 --config=lichen.yaml \
            /tmp/lichen-bin/ccwhid-linux /tmp/lichen-bin/ccwhid-windows
```

(The freshness gate needs no CI change — `TestGeneratedFileUpToDate` already runs in the test step.)

- [ ] **Step 4: Archive the notices file**

In `.goreleaser.yaml`, extend the archives `files` list:

```yaml
    files:
      - README.md
      - LICENSE
      - THIRD_PARTY_LICENSES.txt
```

- [ ] **Step 5: README — Flags row and Third-party licenses section**

Add to the Flags table (after the `--usage` row):

```markdown
| `--license` | Print third-party license information and exit |
```

Insert directly above the `## License` section:

```markdown
## Third-party licenses

ccwhid builds on these components (direct dependencies; everything is
statically linked into the binary):

| Component | Use | License |
|---|---|---|
| [cobra](https://github.com/spf13/cobra) © The Cobra Authors | CLI framework | [Apache-2.0](https://github.com/spf13/cobra/blob/main/LICENSE.txt) |
| [bubbletea](https://github.com/charmbracelet/bubbletea) © Charmbracelet, Inc | Interactive session-picker TUI | [MIT](https://github.com/charmbracelet/bubbletea/blob/main/LICENSE) |
| [lipgloss](https://github.com/charmbracelet/lipgloss) © Charmbracelet, Inc | TUI styling | [MIT](https://github.com/charmbracelet/lipgloss/blob/master/LICENSE) |
| [goldmark](https://github.com/yuin/goldmark) © Yusuke Inuzuka | Markdown rendering | [MIT](https://github.com/yuin/goldmark/blob/master/LICENSE) |
| [goldmark-highlighting](https://github.com/yuin/goldmark-highlighting) © Yusuke Inuzuka | Syntax-highlight extension for goldmark | [MIT](https://github.com/yuin/goldmark-highlighting/blob/master/v2/LICENSE) |
| [chroma](https://github.com/alecthomas/chroma) © Alec Thomas | Syntax-highlighting engine | [MIT](https://github.com/alecthomas/chroma/blob/master/COPYING) |

The complete notices — including all transitive dependencies and the Go
standard library — live in [`THIRD_PARTY_LICENSES.txt`](THIRD_PARTY_LICENSES.txt),
ship inside every release archive, and are embedded in the binary itself:
`ccwhid --license` prints them.
```

- [ ] **Step 6: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all PASS.

- [ ] **Step 7: Commit**

```bash
git add lichen.yaml .github/workflows/ci.yml .goreleaser.yaml README.md
git commit -m "ci: lichen license gate; ship THIRD_PARTY_LICENSES.txt in archives; README table"
```
