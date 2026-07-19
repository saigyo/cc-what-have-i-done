# Report Version Link Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the ccwhid build version as a small muted link under the "ccwhid" brand in the report topbar — release versions link to their GitHub release page, everything else shows "dev build" linking to the repo.

**Architecture:** A pure `versionLink` helper in the render package maps the build version string to a (label, href) pair; `render.Options` gains a `Version` field plumbed from `main.version`; the template and CSS stack the link under the brand. Follows the existing pattern where `cmd/ccwhid` passes options into `render.Site`.

**Tech Stack:** Go 1.26, stdlib only; tests with plain `testing`.

**Spec:** `docs/superpowers/specs/2026-07-19-report-version-link-design.md`

## Global Constraints

- Valid release version = exactly `<digits>.<digits>.<digits>`, with an optional leading `v` (GoReleaser strips the tag's `v`, so released binaries carry `1.2.3`). Pre-releases like `1.2.3-rc1` are NOT valid — they render as dev builds.
- Valid → label `v<x.y.z>` (leading `v` always shown), href `https://github.com/saigyo/cc-what-have-i-done/releases/tag/v<x.y.z>`.
- Invalid → label `dev build`, href `https://github.com/saigyo/cc-what-have-i-done/`.
- No regexp — digit-check with plain string operations, matching codebase convention. No new dependencies.
- The version line appears on every page (index + subagent pages share the template).
- Run `gofmt -l internal/ cmd/` before each commit; it must print nothing.
- Every commit message ends with the two trailer lines shown in Task 1 Step 5 (Co-Authored-By with the implementer's own model name + Claude-Session).

---

### Task 1: versionLink helper

**Files:**
- Modify: `internal/render/format.go`
- Test: `internal/render/format_test.go`

**Interfaces:**
- Consumes: nothing new — pure function over its argument.
- Produces: `versionLink(version string) (label, href string)` and the package-level `const repoURL = "https://github.com/saigyo/cc-what-have-i-done"`. Task 2 calls `versionLink` from `buildViewModel`.

- [ ] **Step 1: Write the failing test**

Append to `internal/render/format_test.go`:

```go
func TestVersionLink(t *testing.T) {
	const repo = "https://github.com/saigyo/cc-what-have-i-done/"
	const tag = "https://github.com/saigyo/cc-what-have-i-done/releases/tag/"
	cases := []struct{ in, label, href string }{
		{"1.2.3", "v1.2.3", tag + "v1.2.3"},
		{"v1.2.3", "v1.2.3", tag + "v1.2.3"},
		{"0.10.7", "v0.10.7", tag + "v0.10.7"},
		{"dev", "dev build", repo},
		{"", "dev build", repo},
		{"1.2.3-rc1", "dev build", repo},
		{"1.2", "dev build", repo},
		{"v1.2.3.4", "dev build", repo},
		{"vx.y.z", "dev build", repo},
		{"1..3", "dev build", repo},
	}
	for _, c := range cases {
		label, href := versionLink(c.in)
		if label != c.label || href != c.href {
			t.Errorf("versionLink(%q) = (%q, %q), want (%q, %q)", c.in, label, href, c.label, c.href)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/ -run TestVersionLink -v`
Expected: compile FAIL — `undefined: versionLink`.

- [ ] **Step 3: Write minimal implementation**

In `internal/render/format.go`, add `"strings"` to the imports and append:

```go
// repoURL is the project home; versionLink points here when no release tag
// can be derived from the build version.
const repoURL = "https://github.com/saigyo/cc-what-have-i-done"

// versionLink maps a build version to the label and href shown under the
// brand in the topbar. "1.2.3" or "v1.2.3" (GoReleaser strips the tag's v
// prefix) yield ("v1.2.3", …/releases/tag/v1.2.3); anything else — dev
// builds, pre-releases, malformed strings — yields ("dev build", the repo).
func versionLink(version string) (label, href string) {
	v := strings.TrimPrefix(version, "v")
	if !isReleaseVersion(v) {
		return "dev build", repoURL + "/"
	}
	return "v" + v, repoURL + "/releases/tag/v" + v
}

// isReleaseVersion reports whether v is exactly <digits>.<digits>.<digits>.
func isReleaseVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return false
			}
		}
	}
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/ -run TestVersionLink -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/render/format.go internal/render/format_test.go
git commit -m "feat(render): versionLink maps build version to release/repo link

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

### Task 2: Plumb the version into the topbar

**Files:**
- Modify: `internal/render/render.go` (Options ~line 21, viewData ~line 131, buildViewModel ~line 180)
- Modify: `internal/render/assets/report.html.tmpl` (brand div ~line 12)
- Modify: `internal/render/assets/styles.css` (`.brand` rule, line 29)
- Modify: `cmd/ccwhid/run.go:110`
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `versionLink(version string) (label, href string)` from Task 1; existing `Site`, `sampleSession()` test helper, `main.version` (package variable in `cmd/ccwhid/main.go`, defaults to `"dev"`).
- Produces: `render.Options.Version string`; `viewData.VersionLabel` / `viewData.VersionHref` strings consumed by the template.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go`:

```go
func TestSiteShowsReleaseVersionLink(t *testing.T) {
	dir := t.TempDir()
	if err := Site(sampleSession(), dir, Options{Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if !strings.Contains(s, `href="https://github.com/saigyo/cc-what-have-i-done/releases/tag/v1.2.3"`) {
		t.Errorf("index.html missing release link")
	}
	if !strings.Contains(s, `class="brand-version"`) || !strings.Contains(s, ">v1.2.3</a>") {
		t.Errorf("index.html missing version label")
	}
}

func TestSiteShowsDevBuildLink(t *testing.T) {
	dir := t.TempDir()
	if err := Site(sampleSession(), dir, Options{Version: "dev"}); err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	s := string(html)
	if !strings.Contains(s, `href="https://github.com/saigyo/cc-what-have-i-done/"`) {
		t.Errorf("index.html missing repo link")
	}
	if !strings.Contains(s, ">dev build</a>") {
		t.Errorf("index.html missing dev build label")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestSiteShows' -v`
Expected: compile FAIL — `unknown field Version in struct literal of type Options`.

- [ ] **Step 3: Implement the wiring**

In `internal/render/render.go`, extend `Options`:

```go
// Options configures a render.
type Options struct {
	Title   string
	Usage   bool   // render the token-usage & cost section
	Version string // ccwhid build version; shown under the brand ("" → dev build)
}
```

Extend `viewData` — add two fields after `Subtitle`:

```go
	Subtitle     string
	VersionLabel string
	VersionHref  string
```

In `buildViewModel`, after the `viewData` literal is built (directly after the closing brace of the `d := viewData{...}` assignment):

```go
	d.VersionLabel, d.VersionHref = versionLink(opts.Version)
```

In `internal/render/assets/report.html.tmpl`, replace

```html
    <div class="brand">ccwhid</div>
```

with

```html
    <div class="brand">ccwhid <a class="brand-version" href="{{ .VersionHref }}">{{ .VersionLabel }}</a></div>
```

(The space between `ccwhid` and the anchor is load-bearing: it keeps the
brand and version as separate DOM text nodes so extracted text reads
"ccwhid dev build", not "ccwhiddev build". Whitespace-only text nodes are
not flex items, so the stacked layout is unaffected.)

In `internal/render/assets/styles.css`, replace line 29

```css
.brand { font-weight: 700; color: var(--accent); letter-spacing: .02em; }
```

with

```css
.brand { font-weight: 700; color: var(--accent); letter-spacing: .02em; display: flex; flex-direction: column; line-height: 1.2; }
.brand-version { font-size: .65rem; font-weight: 400; letter-spacing: 0; color: var(--muted); text-decoration: none; }
.brand-version:hover { text-decoration: underline; }
```

In `cmd/ccwhid/run.go:110`, replace

```go
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage}); err != nil {
```

with

```go
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage, Version: version}); err != nil {
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./...`
Expected: all packages PASS (the two new tests plus the full suite; no existing test asserts the old single-line brand markup).

- [ ] **Step 5: Commit**

```bash
gofmt -l internal/ cmd/   # must print nothing
git add internal/render/render.go internal/render/assets/report.html.tmpl internal/render/assets/styles.css cmd/ccwhid/run.go internal/render/render_test.go
git commit -m "feat(report): version link under the brand in the topbar

Release builds link to their GitHub release page; dev builds show
'dev build' linking to the repo.

Co-Authored-By: <your own model name> <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_017JRPPvmUx8Ht2hWEHMLEG8"
```

---

## Verification (after all tasks)

- `go test ./...` — full suite green; `gofmt -l internal/ cmd/` — no output.
- Manual smoke: `go run ./cmd/ccwhid --latest --out /tmp/ccwhid-version-check --force` and confirm the topbar shows a small "dev build" link under "ccwhid" pointing at the repo (local builds have `version = "dev"`). Subagent pages (if any) show it too.
