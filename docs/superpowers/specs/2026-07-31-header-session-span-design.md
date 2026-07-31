# Header Session Span & Render Time — Design

**Status:** approved (2026-07-31)

**Goal:** The report header's meta line shows when the session ended and
when the report was generated, alongside the existing start time:
`2026-07-03 17:03 → 2026-07-31 19:12   rendered 2026-07-31 20:52`.

## Decisions

- **Layout:** start and last-message time joined as a compact range
  (`→`); only the generation time gets a word label (`rendered`). A hover
  title on the range span explains the arrow.
- **Format:** all three timestamps use the existing meta-line format
  `2006-01-02 15:04`, converted with `.Local()` at render time.
- **Data:** `model.Session.EndedAt` is already populated by the parser
  (last timestamped record wins) — no parser or model changes.

## Design

### View model (`internal/render/render.go`)

`viewData.StartedAt` is replaced by two fields, built in `buildViewModel`:

- `SessionSpan string` — `"2026-07-03 17:03 → 2026-07-31 19:12"`.
  Degradations, in order:
  - `StartedAt` zero → `""` (span omitted, exactly like today's empty
    `StartedAt`).
  - `EndedAt` zero, or start and end format to the identical string
    (sub-minute session) → just the formatted start, no arrow.
- `GeneratedAt string` — the render timestamp, same format, always set.

Time source for `GeneratedAt`: a package-level hook

```go
// timeNow is stubbed in tests to pin the render timestamp.
var timeNow = time.Now
```

so tests can pin it deterministically (override + restore via
`t.Cleanup`).

### Template (`assets/report.html.tmpl`)

The `<span>{{ .StartedAt }}</span>` in `.meta` becomes:

```html
{{ if .SessionSpan }}<span title="session start → last message">{{ .SessionSpan }}</span>{{ end }}
<span>rendered {{ .GeneratedAt }}</span>
```

placed where the start-time span sits today (between the branch and the
turn count). Subagent pages share the code path: their span comes from
the agent session's own start/end; the rendered time is naturally the
same on every page of one report.

### No CSS/JS changes

The `.meta` row already flex-wraps its spans.

## Error handling

- Zero `StartedAt` (no timestamped records): no range span; the
  `rendered` span still appears.
- `EndedAt` before `StartedAt` cannot occur from the parser (both derive
  from the same monotonic record scan); no special handling.

## Testing

- `buildViewModel` tests: full range across two instants; sub-minute
  session collapses to the bare start; zero `EndedAt` collapses to the
  bare start; zero `StartedAt` yields `""`; `GeneratedAt` pinned via the
  `timeNow` override — construct inputs in UTC and assert local-time
  output (same pattern as the `StartedAt` locality test).
- E2E render test: output HTML contains the range span with its
  `title="session start → last message"` attribute and the
  `rendered <timestamp>` span.

## Out of scope

- Session duration text (`Duration()` exists but stays unused here).
- Changing the day separators, turn cards, or sidebar.
- Timezone display or configuration.
