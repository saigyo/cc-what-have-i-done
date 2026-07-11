# ccwhid (cc-what-have-i-done) — Design

**Date:** 2026-07-11
**Status:** Approved

## 1. Purpose & Scope

A single, self-contained Go binary that turns a Claude Code session transcript
(`.jsonl`) into a browsable static HTML report styled like Claude's docs /
artifacts. Run it with flags to target a specific session, or with no args to
open a TUI session browser. Output is a self-contained folder that works over
`file://`, needs no server, and is safe to commit into a project repo as a
documentation artifact.

The report answers: "what did I prompt, and what did Claude Code do?"

## 2. Architecture (packages)

```
cmd/ccwhid/main.go        wiring: cobra root command, flag parsing, dispatch
internal/discovery        find & decode ~/.claude/projects, list sessions + metadata
internal/transcript       parse JSONL into the typed domain model
internal/model            domain types: Session, Turn, Message, ToolCall, Diff, Subagent
internal/redact           secret/path redaction (pluggable rules)
internal/render           build the HTML site from the model (html/template + goldmark + chroma)
internal/assets           go:embed of templates, css, js
internal/tui              Bubble Tea session browser + options screen
```

Boundaries: `transcript` produces a `model.Session` and is the only package that
knows the JSONL wire format. `render` consumes `model.Session` and knows nothing
about JSONL. `tui` orchestrates only — it calls `discovery` and `render`, and
contains no parsing or HTML logic. Each package is independently testable.

**External libraries:** cobra (CLI), Bubble Tea + Bubbles + Lipgloss (TUI),
goldmark (markdown → HTML), chroma (syntax highlighting). All HTML/CSS/JS assets
embedded via `go:embed`. No external fonts or CDNs at runtime.

## 3. Transcript Format (as observed)

Sessions live at `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`. The
encoded directory name is the absolute cwd with `/` replaced by `-` (e.g.
`-Users-markus-IdeaProjects-cc-what-have-i-done`).

Each line is a JSON object with a `type`:
- `user` / `assistant` — carry a `message` field in Anthropic message format
  (content blocks: `text`, `thinking`, `tool_use`, `tool_result`). Threaded via
  `parentUuid`/`uuid`. Metadata: `timestamp`, `cwd`, `gitBranch`, `slug`,
  `version`, `isSidechain`, `isMeta`, `userType`, `toolUseResult`.
- `attachment` — injected attachments.
- `ai-title` — a human-readable session title (`aiTitle`).
- `last-prompt`, `mode`, `permission-mode`, `file-history-snapshot` — session
  state records.

**Subagents:** Task-tool subagent activity appears as records with
`isSidechain: true` in the *same* file, linked to the spawning tool call. These
are folded into the parent `Task` tool call as nested subagent runs.

**Robustness:** the parser must tolerate unknown `type` values, unknown content
block kinds, and malformed lines (see §9).

## 4. Data Flow

1. `discovery` scans `~/.claude/projects/*/*.jsonl`, decodes project paths, and
   builds a lightweight index per session: id, project path, `ai-title`, first
   user prompt (fallback title), start/end timestamps, message count,
   has-subagents flag. (Reads cheaply — enough for listing without full parse.)
2. `transcript` parses the chosen file: folds the flat `parentUuid`-threaded
   records into an ordered **timeline of turns** (user prompt → assistant
   text/thinking/tool_use → tool_result). Sidechain runs attach to their parent
   `Task` tool call as nested `Subagent`s. Meta/system-reminder noise is dropped
   per the curated setting.
3. `redact` walks user-visible text fields applying redaction rules (on by
   default).
4. `render` emits `index.html` + `assets/` into the output directory.

## 5. Rendering Model (curated & readable)

- **User prompts** — rendered as markdown, visually distinct (accent-tinted
  card). These are the spine of the report.
- **Assistant text** — markdown → HTML via goldmark; fenced code blocks
  syntax-highlighted via chroma.
- **Tool calls** — a compact card per call: icon + tool name + one-line summary
  (e.g. `Edit  src/foo.go`, `Bash  git status`). Full command/args/result body
  is collapsible, **collapsed by default**. `Edit`/`Write` render as
  syntax-highlighted diffs.
- **Thinking blocks** — collapsed, muted styling, labeled "thinking".
- **Subagents** — nested collapsible block under the spawning `Task` call.
- **Hidden by default** — `system-reminder` content, `attachment` records,
  `isMeta` records. (A reveal toggle for these is out of scope for v1.)

## 6. Site Structure & Interactivity (light vanilla JS)

```
<out>/index.html          the session report
<out>/assets/styles.css
<out>/assets/app.js        collapse toggles, client-side filter/search, sidebar nav
```

- **Left sidebar** — jump-to-prompt list of the session's user turns (the "what
  did I ask" index).
- **Top bar** — text filter (hides non-matching turns), expand/collapse-all, and
  a toggle to show/hide tool detail.
- All interactivity is client-side vanilla JS, works over `file://`, no server.
- Long sessions render as one page. Message virtualization/pagination is out of
  scope for v1.

## 7. Styling (Claude docs / artifacts vibe)

Warm near-white background (≈`#F5F4EE`), high-contrast text, Claude coral/rust
accent (≈`#D97757`), rounded cards with soft borders, generous whitespace,
clean sans body with monospace for code. Ships **light + dark** themes
(respects `prefers-color-scheme` with a manual toggle). All CSS embedded — no
external fonts or CDNs (offline longevity).

## 8. CLI Surface

```
ccwhid                         no args → TUI browser
ccwhid --session <id|prefix>   render a specific session (id or unambiguous prefix)
ccwhid --project <path|name>   scope to a project (with --session or --latest)
ccwhid --latest                most recent session (of project, or globally)
ccwhid --out <dir>             output folder (default: ./ccwhid-report/<session-short>)
ccwhid --title <str>           override report title
ccwhid --include-subagents     default true; --no-include-subagents to omit
ccwhid --no-redact             disable redaction
ccwhid --force                 overwrite a non-empty output directory
ccwhid --open                  open the result in a browser after generating
```

Built with cobra. The TUI mirrors these: browse projects → sessions (grouped,
current project on top), then a small options screen (output dir, include
subagents, redact, open-after) before generating.

## 9. Error Handling

- Malformed JSONL lines are skipped with a counted warning; never abort the whole
  report.
- Missing `~/.claude/projects` → friendly, actionable message.
- Unknown tool types / content blocks render generically (name + raw JSON in a
  collapsible) rather than failing.
- Output directory collision (non-empty) → refuse unless `--force`.

## 10. Redaction (on by default)

`internal/redact` applies rules to all rendered text:
- Common secret shapes: AWS access keys, `sk-`/`ghp_`-style tokens, JWTs,
  generic `KEY=…` / `TOKEN=…` / `SECRET=…` assignments, high-entropy blobs.
- Rewrites the user's home directory to `~`.
- Matches replaced with `[REDACTED:<kind>]`.

`--no-redact` disables. Rules live in one file, easy to extend and tune. Redaction
is best-effort defense-in-depth, not a guarantee; this is documented for users.

## 11. Testing (TDD)

- `transcript` — golden-file tests over small crafted `.jsonl` fixtures (normal
  turn, tool call, diff, sidechain, malformed line, unknown type).
- `redact` — table tests per rule (positive and negative cases).
- `render` — snapshot tests asserting key HTML structure (not pixel styling).
- `discovery` — tested against a temp fake `.claude` tree.
- `tui` — list-building and filtering logic unit-tested; the interactive layer
  kept thin.

## 12. Out of Scope for v1 (YAGNI)

Multi-session/combined reports, PDF export, live-follow of an active session,
cross-project full-text search index, message virtualization, config files.
All addable later without reworking the core model.
