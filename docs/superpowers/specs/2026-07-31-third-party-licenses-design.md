# Third-Party Licenses — README Table + `--license` Flag — Design

**Status:** approved (2026-07-31)

**Goal:** Document the licenses of ccwhid's direct dependencies in the
README, and let the shipped binary print the same information (with full
license texts) via a `--license` CLI flag.

## Motivation

ccwhid statically links six third-party libraries into its release
binaries. Their licenses (MIT, Apache-2.0) require the copyright and
permission notices to accompany distributed copies. A README table makes
the attribution visible in the repo; the `--license` flag makes it
self-contained in the binary — no repo access needed.

## Scope

**Direct dependencies only** — the six modules in go.mod's first `require`
block:

| Module | License | Copyright |
|---|---|---|
| github.com/alecthomas/chroma/v2 | MIT | Alec Thomas |
| github.com/charmbracelet/bubbletea | MIT | Charmbracelet, Inc |
| github.com/charmbracelet/lipgloss | MIT | Charmbracelet, Inc |
| github.com/spf13/cobra | Apache-2.0 | The Cobra Authors |
| github.com/yuin/goldmark | MIT | Yusuke Inuzuka |
| github.com/yuin/goldmark-highlighting/v2 | MIT | Yusuke Inuzuka |

Indirect dependencies are out of scope by decision: the README and the
flag both say "direct dependencies" explicitly.

## Decisions

- **Output form:** `--license` prints a compact aligned table first
  (Component / Version / License / Copyright), then the six full license
  texts. Full texts make binary distribution formally compliant; ~25 KB
  embedded is negligible.
- **Drift guard:** license file copies live in the repo and are embedded
  via `go:embed`; a test parses go.mod and fails when the direct-require
  set and the component list diverge. Dependency changes break CI until
  data and README are updated.
- **Versions:** resolved at runtime from `runtime/debug.ReadBuildInfo()`,
  matched by module path — always what was actually compiled in, zero
  maintenance. `(unknown)` when build info lacks the module (e.g. some
  non-module test builds).
- **README versions:** none — go.mod owns version truth; the README table
  lists component, use, and license only (ayaki style).

## Design

### New package `internal/licenses`

- `internal/licenses/data/` — verbatim copies of each dependency's license
  file, named by module path with `/` → `_` and no version suffix:
  - `github.com_alecthomas_chroma_v2.txt` (from COPYING)
  - `github.com_charmbracelet_bubbletea.txt`
  - `github.com_charmbracelet_lipgloss.txt`
  - `github.com_spf13_cobra.txt` (from LICENSE.txt)
  - `github.com_yuin_goldmark.txt`
  - `github.com_yuin_goldmark-highlighting_v2.txt`
- `licenses.go`:
  - `//go:embed data` filesystem.
  - `type component struct { Module, Repo, Copyright, License, Use, file string }`
  - `var components = [...]component{ ... }` — one entry per direct
    dependency, ordered alphabetically by module path. `file` is the
    data/ filename.
  - `func Report(w io.Writer) error` — writes:
    1. Header line: `ccwhid incorporates the following third-party
       components (direct dependencies):`
    2. Aligned text table: Component, Version, License, Copyright.
       Column widths computed from content (text/tabwriter is fine and
       stdlib).
    3. For each component: a `── <module path> ──` separator line
       followed by the embedded license text.
  - `func moduleVersions() map[string]string` — from
    `debug.ReadBuildInfo()`; missing info or missing module → `(unknown)`.

### CLI (`cmd/ccwhid/main.go`)

- `options` gains `license bool`; flag registration:
  `f.BoolVar(&opts.license, "license", false, "print third-party license
  information and exit")`.
- At the top of `run`, before validation and session resolution:
  `if opts.license { return licenses.Report(cmd.OutOrStdout()) }`.

### README

New `## Third-party licenses` section (after the Redaction section),
ayaki-style:

> ccwhid builds on third-party components under their own licenses
> (direct dependencies; all statically linked into the binary):

| Component | Use | License |
|---|---|---|
| [cobra](https://github.com/spf13/cobra) © The Cobra Authors | CLI framework | [Apache-2.0](https://github.com/spf13/cobra/blob/main/LICENSE.txt) |
| [bubbletea](https://github.com/charmbracelet/bubbletea) © Charmbracelet, Inc | Interactive session-picker TUI | [MIT](https://github.com/charmbracelet/bubbletea/blob/main/LICENSE) |
| [lipgloss](https://github.com/charmbracelet/lipgloss) © Charmbracelet, Inc | TUI styling | [MIT](https://github.com/charmbracelet/lipgloss/blob/master/LICENSE) |
| [goldmark](https://github.com/yuin/goldmark) © Yusuke Inuzuka | Markdown rendering | [MIT](https://github.com/yuin/goldmark/blob/master/LICENSE) |
| [goldmark-highlighting](https://github.com/yuin/goldmark-highlighting) © Yusuke Inuzuka | Syntax-highlight extension for goldmark | [MIT](https://github.com/yuin/goldmark-highlighting/blob/master/v2/LICENSE) |
| [chroma](https://github.com/alecthomas/chroma) © Alec Thomas | Syntax-highlighting engine | [MIT](https://github.com/alecthomas/chroma/blob/master/COPYING) |

Closing line: `ccwhid --license` prints this information, including the
full license texts, from the binary itself.

Row order is by role (CLI, TUI, rendering); the code's `components` slice
is alphabetical by module path. One consistent principle per surface.

## Drift guard (test in `internal/licenses`)

- Parse `../../go.mod` (via `runtime.Caller`-relative or plain relative
  path from the package directory): collect module paths inside the first
  `require (` … `)` block, skipping lines containing `// indirect`.
  Plain line scanning — no new dependency for go.mod parsing.
- Assert the set equals the module paths in `components` (both
  directions: missing and extra are failures, with the offending paths in
  the message).
- Assert every embedded license file is non-empty and contains either
  "Copyright" or "License".

## Error handling

- `Report` propagates writer errors; embedded reads cannot fail at
  runtime (embed is compile-time verified — a missing file in `data/`
  fails the build).
- `--license` combined with other flags: other flags are ignored
  (short-circuit before validation), matching `--version` behavior. Not
  an error.

## Testing

- `internal/licenses`: Report output contains all six module paths, the
  table header, and each license text's first line; drift test as above;
  version fallback test (`moduleVersions` on a test binary → entries
  resolve or `(unknown)`, no panic).
- `cmd/ccwhid`: flag-list test gains `license`; a test that executing the
  root command with `--license` writes the header line to stdout and does
  not attempt session discovery (exit nil without a session).

## Out of scope

- Indirect dependencies (both surfaces say "direct").
- Go standard library / toolchain attribution.
- SPDX machine-readable output, JSON, SBOM formats.
- Automating the license-file copies (six files, guarded by CI; manual
  update on the rare direct-dep change is fine).
