# Sidebar Prompt Times & Date Dividers — Design

**Status:** approved (2026-07-31)

**Goal:** The sidebar prompt list mirrors the transcript's new time
treatment: each prompt entry shows its local time right-aligned on the
row, and compact date dividers segment the list by calendar day —
follow-on to the turn time labels / day segmentation feature.

## Decisions

- **Times:** same values as the turn cards — `15:04` label, full
  `January 2, 2006 at 15:04:05` hover title — right-aligned in their own
  column so the preview keeps maximum width.
- **Dividers:** compact `January 2, 2006` text (no weekday — the sidebar
  is a narrow 220px column); hovering shows the full
  `Monday, January 2, 2006` form.
- **Numbering:** prompt numbers are unchanged — the list keeps its global
  CSS counter and divider items opt out.
- **Independence:** sidebar day detection runs over prompts only. A day
  that has turns but no prompts gets no sidebar divider; the main pane's
  separators are unaffected.

## Design

### View model (`internal/render/render.go`)

`promptRef` (currently `Index`, `Preview`) gains four string fields,
computed in the existing `buildViewModel` loop from the user turn's
`Timestamp`, reusing `timeParts`:

- `TimeLabel` — `"15:04"` format; `""` when the turn has no timestamp.
- `TimeTitle` — `"January 2, 2006 at 15:04:05"` format, hover title.
- `DayHeader` — `"January 2, 2006"` format, set only on the first
  timestamped prompt of each local calendar day.
- `DayTitle` — `"Monday, January 2, 2006"` format, set together with
  `DayHeader`, shown as the divider's hover title.

Day detection uses its own tracker (last seen local date across prompts),
separate from the turn-card tracker. Zero-timestamp prompts get no time,
no divider, and do not participate in day-change detection.

### Template (`assets/report.html.tmpl`)

The sidebar loop becomes:

```html
{{ range .Prompts }}{{ if .DayHeader }}<li class="prompt-day" title="{{ .DayTitle }}">{{ .DayHeader }}</li>{{ end }}<li><a href="#turn-{{ .Index }}"><span class="prompt-preview">{{ .Preview }}</span>{{ if .TimeLabel }}<span class="prompt-time" title="{{ .TimeTitle }}">{{ .TimeLabel }}</span>{{ end }}</a></li>{{ end }}
```

### Styling (`assets/styles.css`)

- `.prompt-list li.prompt-day`: `counter-increment: none` (overrides the
  list's `counter-increment: p`), small muted uppercase styling mirroring
  `.sidebar-title`, top margin to separate day groups, `cursor: default`.
- `.prompt-list a`: becomes a flex row (`display: flex`,
  `align-items: baseline`, small gap); `.prompt-preview` takes the
  remaining width (`flex: 1`) and wraps exactly as previews do today;
  `.prompt-time` sits right-aligned on the first line, muted, `.7rem`,
  `white-space: nowrap`.
- The existing `.prompt-list a::before` number counter is unaffected.

### No JS changes

The turn filter never touches the sidebar; dividers and times are static.

## Error handling

- Prompts without timestamps render exactly as today (no time span, no
  divider), under whichever divider last appeared.
- A session whose prompts all lack timestamps renders no dividers and no
  time spans — the sidebar is byte-identical to the current output.

## Testing

- `buildViewModel` test: prompts spanning two local days → `DayHeader`/
  `DayTitle` set exactly on the first prompt of each day; `TimeLabel`/
  `TimeTitle` set on timestamped prompts; a zero-timestamp prompt gets
  none of the four and does not reset detection.
- E2E render test: sidebar HTML contains `prompt-day` (with `title`) and
  `prompt-time` (with `title`) elements.
- Formats asserted with expectations built via `time.Local` construction
  so tests pass in any timezone (same pattern as the turn-card tests).

## Out of scope

- Filtering or collapsing the sidebar by day.
- Any change to the main pane's day separators or turn cards.
- Relative times or durations.
