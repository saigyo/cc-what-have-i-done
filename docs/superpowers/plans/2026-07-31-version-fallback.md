# Version Fallback for `go install` Builds Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Binaries built with `go install …/cmd/ccwhid@vX.Y.Z` report the real version (in `--version` and the report topbar link) instead of `dev`.

**Architecture:** Keep `var version = "dev"` as GoReleaser's ldflags target. A pure `resolveVersion(ldflags, mainVersion)` helper picks the ldflags value when set, else the module version Go stamped into the binary (`debug.ReadBuildInfo().Main.Version`, `v` prefix stripped), else `"dev"`. The resolved value is computed once in `newRootCmd`, stored on the existing `options` struct, and feeds both cobra's `Version` and `render.Options.Version`.

**Tech Stack:** Go stdlib only (`runtime/debug`, `strings`); plain `testing`.

## Global Constraints

- No new go.mod dependencies; no changes to `internal/render` or the release pipeline (GoReleaser ldflags stay as-is).
- Precedence, exactly: non-`"dev"` ldflags value wins verbatim; else a non-empty, non-`"(devel)"` build-info main version with `"v"` prefix stripped; else `"dev"`.
- All code gofmt-clean, `go vet ./...` clean, tests pass with `-race`.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

---

### Task 1: resolveVersion helper + wiring

**Files:**
- Modify: `cmd/ccwhid/main.go` (imports at :3-12, version var at :16, `options` struct at :18-33, `newRootCmd` at :43)
- Modify: `cmd/ccwhid/run.go:110` (the `render.Site` call)
- Test: `cmd/ccwhid/main_test.go`

**Interfaces:**
- Consumes: `var version = "dev"` (main.go:16, ldflags target — unchanged); `debug.ReadBuildInfo()` from `runtime/debug`.
- Produces: `resolveVersion(ldflags, mainVersion string) string` and `buildVersion() string` in package main; `options.version string` field carrying the resolved value.

- [ ] **Step 1: Write the failing test**

Append to `cmd/ccwhid/main_test.go`:

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/ccwhid/ -run TestResolveVersion`
Expected: FAIL to build — `undefined: resolveVersion`.

- [ ] **Step 3: Implement helper and wiring**

In `cmd/ccwhid/main.go`:

1. Add `"runtime/debug"` and `"strings"` to the stdlib import group.

2. Directly below `var version = "dev"` (keep the var and its comment unchanged), add:

```go
// resolveVersion picks the version to report: the GoReleaser ldflags
// value when set, else the module version Go stamped into the binary
// (go install m@vX.Y.Z, or a VCS-derived version), else "dev". The "v"
// prefix is stripped to match GoReleaser's convention.
func resolveVersion(ldflags, mainVersion string) string {
	if ldflags != "dev" {
		return ldflags
	}
	if mainVersion == "" || mainVersion == "(devel)" {
		return ldflags
	}
	return strings.TrimPrefix(mainVersion, "v")
}

// buildVersion resolves the version for this running binary.
func buildVersion() string {
	mv := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		mv = info.Main.Version
	}
	return resolveVersion(version, mv)
}
```

3. Add a `version` field to the `options` struct, after `license bool`:

```go
	version          string
```

4. In `newRootCmd`, resolve once and use it for cobra (replace `Version: version,`):

```go
	opts := &options{version: buildVersion()}
	cmd := &cobra.Command{
		...
		Version:      opts.version,
```

(The `opts := &options{}` line already exists at the top of `newRootCmd` — change it to `opts := &options{version: buildVersion()}`; then change the `Version: version,` line to `Version: opts.version,`.)

In `cmd/ccwhid/run.go` line 110, change `Version: version` to `Version: opts.version`:

```go
	if err := render.Site(sess, outDir, render.Options{Title: opts.title, Usage: opts.usage, Version: opts.version, NoImages: opts.noImages}); err != nil {
```

- [ ] **Step 4: Run the tests**

Run: `go test ./cmd/ccwhid/ -race`
Expected: PASS (all, including TestResolveVersion and the existing flag/help/license tests).

- [ ] **Step 5: Verify behavior on a real build**

Run: `go build -o /tmp/ccwhid-vtest ./cmd/ccwhid && /tmp/ccwhid-vtest --version; go build -ldflags "-X main.version=9.9.9" -o /tmp/ccwhid-vtest ./cmd/ccwhid && /tmp/ccwhid-vtest --version; rm /tmp/ccwhid-vtest`
Expected: first line reports a VCS-derived version (e.g. `ccwhid version 0.6.0` at a clean tag, or a pseudo-version / `dev` on a dirty tree — anything but a wrong hardcoded value); second line reports exactly `ccwhid version 9.9.9` (ldflags still win).

- [ ] **Step 6: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/ccwhid/main.go cmd/ccwhid/run.go cmd/ccwhid/main_test.go
git commit -m "fix(version): report module version for go-install builds"
```
