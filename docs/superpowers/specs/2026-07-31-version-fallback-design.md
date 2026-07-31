# Version Fallback for `go install` Builds — Design

**Status:** approved (2026-07-31)

**Goal:** Binaries installed via `go install
github.com/saigyo/cc-what-have-i-done/cmd/ccwhid@vX.Y.Z` report their real
version instead of `dev` — in `--version` output and in the report
topbar's version link.

## Problem

`var version = "dev"` in `cmd/ccwhid/main.go` is overridden only by
GoReleaser's `-ldflags "-X main.version=…"`. `go install module@version`
compiles without those flags, so a user installing the tagged v0.6.0
still sees `ccwhid version dev`, and reports rendered by that binary link
to the repo ("dev build") instead of the release page.

## Decision

Ldflags-first with a `runtime/debug` build-info fallback (standard Go
pattern). No release-pipeline changes; GoReleaser behavior is unchanged.

## Design

In `cmd/ccwhid/main.go`:

- Keep `var version = "dev"` as the ldflags injection target.
- Add a pure, testable helper:

  ```go
  // resolveVersion picks the version to report: the GoReleaser ldflags
  // value when set, else the module version Go stamped into the binary
  // (go install m@vX.Y.Z, or a VCS-derived version), else "dev".
  // The "v" prefix is stripped to match GoReleaser's convention.
  func resolveVersion(ldflags, mainVersion string) string {
      if ldflags != "dev" {
          return ldflags
      }
      if mainVersion == "" || mainVersion == "(devel)" {
          return ldflags
      }
      return strings.TrimPrefix(mainVersion, "v")
  }
  ```

- Wire it where `version` is consumed, reading build info once:

  ```go
  func buildVersion() string {
      mv := ""
      if info, ok := debug.ReadBuildInfo(); ok {
          mv = info.Main.Version
      }
      return resolveVersion(version, mv)
  }
  ```

  `newRootCmd` sets `Version: buildVersion()` and `run` passes the same
  value into `render.Options.Version` (store it once in `newRootCmd` and
  thread it via the existing `opts` struct as a new field, so cobra
  output and the report always agree).

## Resulting behavior

| Build | Reported version | Report topbar |
|---|---|---|
| GoReleaser release | `0.6.0` (ldflags, unchanged) | release-page link |
| `go install …@v0.6.0` | `0.6.0` (build info, v stripped) | release-page link |
| `go build` from clean checkout at tag (Go ≥1.24 VCS stamping) | `0.6.0` | release-page link |
| `go build` with dirty/untagged tree | pseudo-version or `dev` | "dev build" → repo (pseudo-versions fail `isReleaseVersion`) |
| No build info (e.g. some test binaries) | `dev` | "dev build" → repo |

No changes to `internal/render` — `versionLink` already accepts both
`0.6.0` and `v0.6.0` and routes non-release strings to "dev build".

## Testing

- Unit tests for `resolveVersion` in `cmd/ccwhid/main_test.go`:
  ldflags set (`"0.6.0"`, any mainVersion) → `0.6.0`;
  ldflags `"dev"` + mainVersion `"v0.6.0"` → `0.6.0`;
  ldflags `"dev"` + mainVersion `"(devel)"` → `dev`;
  ldflags `"dev"` + mainVersion `""` → `dev`.
- Existing render tests already cover `versionLink` for both shapes.
- Manual verification after merge/release: `go install …@vX.Y.Z` reports
  the tag version.

## Out of scope

- Changing GoReleaser ldflags or release pipeline.
- Showing commit hashes / build dates for dev builds.
- README changes (install instructions become correct without edits).
