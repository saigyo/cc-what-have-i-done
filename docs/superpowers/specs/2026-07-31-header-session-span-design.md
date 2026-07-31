# Header Session Span & Render Time — Design

**Status:** approved (2026-07-31)

**Goal:** The report header's meta line shows when the session ended and
when the report was generated, alongside the existing start time.
Same-day session: `2026-07-03 17:03 (3h 12m)   rendered 2026-07-31 20:52`.
Cross-day session: `2026-07-03 17:03 → 2026-07-31 19:12   rendered …`.

## Decisions

- **Same-day sessions** (start and last message on the same local
  calendar day — the common case): a single timestamp plus the session
  duration in hours and minutes, `2026-07-03 17:03 (3h 12m)`; the hover
  title carries the exact last-message time.
- **Cross-day sessions:** start and last-message time joined as a
  compact range (`→`), hover title explains the arrow.
- **Render time:** its own span with the word label `rendered`.
- **Format:** timestamps use the existing meta-line format
  `2006-01-02 15:04`, converted with `.Local()` at render time. Duration
  is truncated to whole minutes: `3h 12m`, or `45m` when under an hour.
- **Data:** `model.Session.EndedAt` is already populated by the parser
  (last timestamped record wins) — no parser or model changes.
  `Session.Duration()` supplies the duration.

## Design

### View model (`internal/render/render.go`)

`viewData.StartedAt` is replaced by three fields, built in
`buildViewModel`:

- `SessionSpan string` — the visible text. Cases, in order:
  1. `StartedAt` zero → `""` (span omitted, like today's empty value).
  2. `EndedAt` zero, or duration truncates to under one minute → the
     formatted start alone.
  3. Same local calendar day → `"<start> (<duration>)"`,
     e.g. `2026-07-03 17:03 (3h 12m)`.
  4. Different local days → `"<start> → <end>"`.
- `SessionSpanTitle string` — the hover title matching the case:
  `"session start"` for a bare start; `"last message <end>"` (e.g.
  `last message 2026-07-03 20:15`) for the same-day form;
  `"session start → last message"` for the cross-day range.
- `GeneratedAt string` — the render timestamp, same format, always set.

Helpers:

```go
// timeNow is stubbed in tests to pin the render timestamp.
var timeNow = time.Now

// formatDuration renders d truncated to whole minutes: "3h 12m", "45m".
func formatDuration(d time.Duration) string
```

Same-day comparison uses the two local `Date()` triples (consistent with
`dayTracker`).

### Template (`assets/report.html.tmpl`)

The `<span>{{ .StartedAt }}</span>` in `.meta` becomes:

```html
{{ if .SessionSpan }}<span title="{{ .SessionSpanTitle }}">{{ .SessionSpan }}</span>{{ end }}
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

- `formatDuration` unit tests: `45m`, `3h 12m`, exact hour (`2h 0m`).
- `buildViewModel` tests, one per case: cross-day range; same-day
  start + duration (visible text and `last message …` title); zero
  `EndedAt` → bare start; sub-minute session → bare start; zero
  `StartedAt` → `""`; `GeneratedAt` pinned via the `timeNow` override —
  construct inputs in UTC and assert local-time output (same pattern as
  the `StartedAt` locality test).
- E2E render test: output HTML contains the session span with a `title`
  attribute and the `rendered <timestamp>` span.

## Out of scope

- Changing the day separators, turn cards, or sidebar.
- Timezone display or configuration.
