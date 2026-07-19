# Report Version Link — Design

**Status:** approved (2026-07-19)

**Goal:** Show the ccwhid version small and unobtrusively in the report
topbar, directly below the "ccwhid" brand title, linking to the matching
GitHub release page — or, for local/dev builds, showing a "dev build"
placeholder linking to the repository.

## Motivation

The binary knows its version (`cmd/ccwhid/main.go`: `var version = "dev"`,
overridden at release time by GoReleaser's
`-ldflags "-X main.version={{ .Version }}"` — note GoReleaser strips the
tag's `v` prefix, so released binaries carry `1.2.3` while the tag is
`v1.2.3`). Reports currently give no hint which ccwhid version produced
them; before cutting a new release, wire this through so every report is
self-identifying.

## Behavior

- **Placement:** in the sticky topbar, a second line below the `ccwhid`
  brand text — small (~0.65rem), muted color, on every page (index and
  subagent pages share the template).
- **Valid version** (matches `x.y.z` digits, with or without a leading
  `v`): label `vX.Y.Z` (leading `v` always shown), linking to
  `https://github.com/saigyo/cc-what-have-i-done/releases/tag/vX.Y.Z`.
- **Anything else** (`dev`, empty, snapshot, pre-releases like
  `1.2.3-rc1`): label `dev build`, linking to
  `https://github.com/saigyo/cc-what-have-i-done/`.
- Pre-releases intentionally count as dev builds (strict `v?\d+.\d+.\d+`
  only).

## Design

### Plumbing

- `render.Options` gains `Version string`.
- `cmd/ccwhid/run.go` (wherever `render.Options` is built) passes
  `main.version` through. `newRootCmd`/`run` wiring follows the existing
  pattern used for the other options.

### Logic (`internal/render`)

A pure helper, unit-testable in isolation:

```go
// versionLink maps a build version to the label and href shown under the
// brand. "1.2.3" or "v1.2.3" → ("v1.2.3", releases/tag/v1.2.3); anything
// else → ("dev build", repo root).
func versionLink(version string) (label, href string)
```

Validation without regexp (matching codebase convention): trim an optional
leading `v`, split on `.`, require exactly three non-empty all-digit parts.
Repo URL is a package-level const: `https://github.com/saigyo/cc-what-have-i-done`.

### View model & template

- `viewData` gains `VersionLabel` and `VersionHref` (plain strings,
  auto-escaped by `html/template`).
- `report.html.tmpl`: the brand div becomes a stacked block:

```html
<div class="brand">ccwhid
  <a class="brand-version" href="{{ .VersionHref }}">{{ .VersionLabel }}</a>
</div>
```

### CSS (`styles.css`)

`.brand` becomes a small flex column; `.brand-version` is ~0.65rem,
`var(--muted)`, no underline (underline on hover), normal letter-spacing
and weight so it stays unobtrusive in both themes.

## Out of scope

- No version in the TUI, `--version` flag output (already exists), or
  README changes.
- No build-info fallback (e.g. `runtime/debug.ReadBuildInfo`) — `dev`
  stays `dev build` locally.

## Testing

- Unit tests for `versionLink`: `"1.2.3"` and `"v1.2.3"` → label
  `v1.2.3` + tag URL; `"dev"`, `""`, `"1.2.3-rc1"`, `"1.2"`, `"v1.2.3.4"`,
  `"vx.y.z"` → `dev build` + repo URL.
- Render test: `Site`/`buildViewModel` with `Options.Version = "1.2.3"`
  produces HTML containing the release href and label; with `"dev"`
  produces the repo href and `dev build`.
