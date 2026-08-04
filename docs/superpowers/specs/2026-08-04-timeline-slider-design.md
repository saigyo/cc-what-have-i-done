# Timeline Slider Design

**Date:** 2026-08-04
**Status:** Approved for planning

## Goal

Very long session reports are hard to navigate. Add a floating timeline slider
at the right edge of the browser window: a thin vertical rail with a draggable
thumb that mirrors scroll position. While dragging the thumb or scrolling the
page, a bubble next to the thumb shows the date and time of the messages
currently in view. Tick marks on the rail show where each day boundary falls.

## Decisions (from brainstorming)

- **Track scale: scroll position.** The track mirrors the document — thumb at
  50% means halfway through the report. Idle time gaps do not create dead
  zones on the track.
- **Bubble visibility: drag + scroll, fades out.** The bubble appears while
  dragging the thumb and while scrolling normally, then fades after ~1 s of
  inactivity.
- **Placement: right viewport edge.** The prompt sidebar occupies the left.
- **Day ticks: yes.** Small marks on the rail at each day boundary, positioned
  by document offset fraction.
- **No minimap density strip, no keyboard navigation for the thumb** (the
  native scrollbar remains fully functional), **no time-proportional track.**

## Architecture

Everything is client-side presentation. The renderer contributes two things:
a machine-usable time label per turn card and a static rail skeleton in the
page. All behavior lives in `app.js`; all appearance in `styles.css`. No new
data pipeline, no changes to parsing or the view model beyond one field.

The slider works identically on the main report page and on agent transcript
pages, since they share the same template and assets.

### 1. Renderer: per-turn time attribute

`turnView` gains one field:

- `TimeBubble string` — the turn's timestamp formatted in render-machine
  local time with Go layout `January 2 · 15:04` (example: `July 29 · 14:03`).
  Empty when the turn has no timestamp.

The template emits it as a data attribute on the turn article, only when
non-empty:

```html
<article class="turn turn-user" id="turn-3" data-time="July 29 · 14:03" …>
```

Rationale for a pre-formatted string instead of an epoch value: the report
renders all times in render-machine local time (Go `.Local()`). Formatting an
epoch in the browser would use the viewer's timezone and could contradict the
card labels. A pre-formatted string keeps the bubble consistent with every
other time in the report by construction.

### 2. Renderer: rail skeleton

The template adds the rail markup once per page, before the closing script
tag:

```html
<div class="timeline hidden" aria-hidden="true">
  <div class="timeline-track">
    <div class="timeline-thumb"></div>
  </div>
  <div class="timeline-bubble"></div>
</div>
```

`aria-hidden="true"`: the rail duplicates the native scrollbar and the bubble
duplicates information already in the cards; screen readers should skip it.
Day tick elements are created by JS inside `.timeline-track`.

### 3. JS behavior (`app.js`)

New self-contained section in the existing IIFE. If `.timeline` is absent
from the DOM, the section does nothing.

**Eligibility.** The rail is shown only when
`document.documentElement.scrollHeight >= 3 * window.innerHeight`, re-checked
on resize and whenever the cached document height changes (see cache below).
Ineligible → the `.timeline` element keeps the `hidden` class. A CSS media
query additionally hides the rail below 760 px viewport width (same
breakpoint that hides the sidebar).

**Thumb sync.** On scroll (requestAnimationFrame-throttled), the thumb's
position along the track is set to
`scrollY / (scrollHeight - innerHeight)`. The thumb has a minimum height so
it stays grabbable on very long documents.

**Dragging.** `pointerdown` on the thumb or track captures the pointer
(`setPointerCapture`). Pointer Y relative to the track maps linearly to a
scroll fraction; `pointermove` calls `window.scrollTo`. A plain click on the
track (not on the thumb) jumps to that fraction. While dragging, the rail
carries a `dragging` class (full opacity, bubble held visible).

**Bubble.** Text = `data-time` of the topmost turn currently in view: the
first non-`.filtered` turn whose bottom edge is below the sticky topbar's
bottom edge. Turns without `data-time` are skipped. The bubble becomes
visible on scroll and on drag, and fades out (CSS opacity transition on a
class toggle) 1000 ms after the last scroll/drag event. If no turn qualifies
(e.g. filter hides everything), the bubble stays hidden.

**Offset cache.** Topmost-turn lookup uses a cached, sorted array of
`{ top, label }` built from all non-`.filtered` turns carrying `data-time`,
searched by binary search per scroll frame. The cache stores the
`scrollHeight` it was built at; every scroll/resize frame compares the live
`scrollHeight` against it and rebuilds on mismatch. This one mechanism covers
filtering, expand/collapse-all, tool-card toggles, individual `<details>`
toggles, and window resizes — anything that changes document height.

**Day ticks.** Rebuilt together with the offset cache: for each
non-`.filtered` `.day-sep`, a `.timeline-tick` div is placed at
`sep.offsetTop / scrollHeight` of the track height, with the day-sep's text
content (weekday form) copied onto the tick as its `title` tooltip.

### 4. CSS (`styles.css`)

- `.timeline` — `position: fixed; right: 16px; top: 4.5rem; bottom: 1rem;
  z-index: 20;` opacity `.55`, transitioning to `1` on hover and while
  `.dragging`. `.timeline.hidden { display: none; }` The 16 px inset keeps
  the rail outside the ~15 px native overlay-scrollbar hit strip at the
  viewport edge, which would otherwise swallow clicks on macOS.
- `.timeline-track` — full height, 8 px wide, rounded, subtle background
  derived from `var(--border)`.
- `.timeline-thumb` — full track width, `min-height: 24px`, rounded,
  `var(--muted)`-ish background, `cursor: grab` (`grabbing` while dragging).
- `.timeline-tick` — absolute 1 px horizontal line across the track,
  `var(--muted)` at reduced opacity.
- `.timeline-bubble` — absolutely positioned to the left of the track,
  vertically aligned with the thumb, chip-styled like existing UI surfaces
  (`var(--surface)` background, `var(--border)` border, small radius,
  `.8rem` font, `white-space: nowrap`), opacity transition for the fade,
  `pointer-events: none`.
- `@media (max-width: 760px) { .timeline { display: none; } }`

Both themes work automatically because all colors come from existing CSS
variables.

## Error handling

- Turns without timestamps: no `data-time` attribute; skipped by the bubble
  lookup. Reports with zero timestamped turns show the rail (if long enough)
  but never show a bubble.
- Filter hides everything: bubble hidden; thumb and drag still work.
- Short reports: rail stays `hidden`; no listeners beyond the cheap
  eligibility check do any work.

## Testing

Go tests (`internal/render`):

1. `buildViewModel` sets `TimeBubble` in `January 2 · 15:04` format for a
   timestamped turn (constructed via `time.Local` per the existing
   timezone-safe pattern) and leaves it empty for a zero-timestamp turn.
2. Rendered page contains `data-time="…"` on timestamped turn articles and
   omits the attribute otherwise.
3. Rendered page contains the rail skeleton (`class="timeline hidden"`,
   `timeline-track`, `timeline-thumb`, `timeline-bubble`) — asserted on the
   full `Site` output so both index and agent pages are covered by the shared
   template.

JS behavior has no test infrastructure in this project; verify manually with
Playwright against the regenerated demo report during development (drag,
scroll fade, day ticks, filter interaction, dark theme).

## Non-goals

- Keyboard navigation for the thumb (native scrolling covers it).
- Time-proportional track scale.
- Minimap density strip / per-turn markers.
- Touch-specific affordances beyond what Pointer Events give for free.
