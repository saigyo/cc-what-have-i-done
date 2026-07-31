# Turn Time Labels & Day Segmentation — Design

**Status:** approved (2026-07-31)

**Goal:** Every turn card in the report carries a time label next to its
role label ("Claude · 14:03", "You · 14:03"); hovering shows the full date
and time; the transcript is segmented by calendar days with centered day
headers — like Claude Code History Viewer.

## Decisions

- **Timezone:** timestamps (stored UTC in the transcript) are converted
  with `.Local()` at render time — the local timezone of the machine
  running ccwhid. Static output, no client-side formatting.
- **Format:** 24-hour. Card label `14:03`; hover title
  `July 29, 2026 at 14:03:59`; day header `Wednesday, July 29, 2026`.
- **All turn kinds:** the label appears on every card — user ("You"),
  assistant ("Claude"), and agent-result ("Agent · <name>") turns alike.
- **Scope:** main page and subagent pages (same code path). No changes to
  parsing, `internal/model`, or the search index.

## Design

### View model (`internal/render/render.go`)

`turnView` gains three string fields, computed in `buildViewModel` from
`t.Timestamp` (already parsed by `internal/transcript`):

- `TimeLabel` — `"15:04"` format, shown next to the role label.
- `TimeTitle` — `"January 2, 2006 at 15:04:05"` format, the hover tooltip.
- `DayHeader` — `"Monday, January 2, 2006"` format, non-empty only on the
  first timestamped turn of each calendar day. The first timestamped turn
  of the session always gets one.

A new pure helper produces the first two:

```go
// timeParts formats a turn timestamp for display: the short card label
// and the full hover title, in local time. Zero time yields "", "".
func timeParts(ts time.Time) (label, title string)
```

Day-change detection lives in the `buildViewModel` loop: track the last
seen local date (`year, month, day`); when a turn's local date differs —
or no date has been seen yet — set `DayHeader` and update the tracker.
Turns with a zero timestamp get empty fields and do not participate in
day-change detection (a zero timestamp never starts or ends a day).

### Consistency fix

The session-head meta line (`viewData.StartedAt`) currently formats the
UTC value directly; it gets the same `.Local()` conversion so the meta
line and card labels agree.

### Template (`assets/report.html.tmpl`)

In the turn loop:

```html
{{ if .DayHeader }}<div class="day-sep"><span>{{ .DayHeader }}</span></div>{{ end }}
<article class="turn turn-{{ .Kind }}" …>
  <div class="turn-role">{{ .RoleLabel }}{{ if .TimeLabel }}<span class="turn-time" title="{{ .TimeTitle }}">· {{ .TimeLabel }}</span>{{ end }}…</div>
```

The time span sits directly after the role label, before the status chip,
agent link, and usage badge. Native `title` attribute for the hover — no
JS, no custom popover.

### Styling (`assets/styles.css`)

- `.day-sep`: flex row, centered muted text (same color token as
  `.turn-role`, `var(--muted)`), horizontal rules on both sides via
  `::before`/`::after` (`flex: 1; border-top: 1px solid var(--border)`
  or the report's existing border token), vertical margins to separate
  day groups.
- `.turn-time`: inherits the `.turn-role` muted styling; digits need no
  `text-transform`; `cursor: default` so the hover affordance reads as
  informational.

### Filter interaction (`assets/app.js`)

The existing filter toggles `.filtered` on `.turn` elements. Addition:
after each filter pass, walk the day separators and hide (same
`.filtered` class) any `.day-sep` that is not followed by at least one
visible `.turn` before the next `.day-sep`. Clearing the filter restores
all separators.

## Error handling

- Zero timestamps (missing/unparseable in the transcript): no time label,
  no tooltip, no day header — the card renders exactly as today.
- A session whose turns all lack timestamps renders no day separators.

## Testing

- Unit tests for `timeParts`: zero time → `"", ""`; a known instant →
  expected label and title (construct expectations with the same
  `time.Local` conversion so tests pass in any timezone).
- `buildViewModel` test: a session with turns spanning two local days →
  `DayHeader` set exactly on the first turn of each day; zero-timestamp
  turns between them get no header and don't reset detection.
- E2E render test: output HTML contains the `turn-time` span with a
  `title` attribute and a `day-sep` element.
- Filter behavior is exercised manually (no JS test harness exists in
  this repo).

## Out of scope

- Client-side (viewer-timezone) formatting.
- Relative times ("2 hours ago"), durations, or per-turn elapsed time.
- Day navigation in the sidebar.
