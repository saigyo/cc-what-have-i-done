# Filter Match Highlighting Design

**Date:** 2026-08-04
**Status:** Approved for planning

## Goal

While the turn filter is active, highlight every occurrence of the query in
the cards that survive the filter — including occurrences inside collapsed
tool-detail sections, so a card that matched on hidden tool output shows its
highlights the moment it is expanded.

## Decisions (from brainstorming)

- **Highlight everywhere in the card**, including inside collapsed
  `<details>`. A match on hidden tool output is the reason a card survived;
  expanding reveals the why.
- **CSS Custom Highlight API** (`CSS.highlights` +
  `::highlight(filter-match)`), not `<mark>` wrapping: zero DOM mutation, so
  nothing to unwrap on query changes, no reflows, and no interaction with the
  timeline slider's height-keyed offset cache and scroll anchoring.
- **Feature detection:** browsers without `CSS.highlights` keep today's
  behavior (filtering without highlights). No polyfill.
- **Minimum query length 2** for highlighting (the filter itself still works
  from 1 character); single-character highlights are noise at this document
  scale.
- **Range cap 1,500** per refresh as a jank guard; occurrences beyond the cap
  are simply not highlighted. (Originally 5,000 — WebKit visibly freezes the
  main thread painting that many ranges.)
- **Forced repaint after every refresh:** WebKit does not repaint regions
  whose ranges were removed from the registry (neither `delete()` nor a
  `set()` replace invalidates them; observed in the DuckDuckGo browser, where
  stale highlights lingered until e.g. a click repainted the word). After
  each `set()`, `<main>` is promoted to its own compositing layer for a
  single frame (`transform: translateZ(0)`, reset on the next
  `requestAnimationFrame`), forcing a rerasterization with the current
  registry state. No layout change; the fixed timeline rail is a sibling of
  `<main>` and unaffected.

## Architecture

Pure client-side presentation. No Go changes, no template changes — the
`::highlight` pseudo-element needs no extra markup. One new self-contained
section in `app.js`, one small block in `styles.css`.

### JS behavior (`app.js`)

New section inside the existing IIFE, guarded by feature detection:

```js
if (typeof CSS !== 'undefined' && CSS.highlights) { … }
```

Absent support → the section registers nothing and does nothing.

**Trigger.** A dedicated listener on `#filter`, registered after the
existing filter handler (so it observes post-toggle `.filtered` classes),
debounced 250 ms with `setTimeout`/`clearTimeout` (long enough that fast
typing skips intermediate prefixes, whose huge match counts are what WebKit
chokes on) — registered for BOTH the
`input` and the `search` event: Safari's native clear gestures (the cancel
button and the Esc key) fire only `search`, not `input`. For the same
reason the existing card-filter handler and the timeline's invalidation
listener also listen to both events; a `search` fired with an unchanged
value (Enter key) is a harmless idempotent re-apply.

**Refresh algorithm.** On each debounced fire:

1. `q = filter.value.trim()`; ranges are collected only when `q.length >= 2`.
2. Build a case-insensitive matcher: escape regex metacharacters in `q`
   (`q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')`), compile with flags `gi`.
   Regex matching (rather than `toLowerCase()` + `indexOf`) keeps match
   indices exact for characters whose lowercase form changes string length.
3. For every `.turn:not(.filtered)` element: iterate its text nodes via
   `document.createTreeWalker(el, NodeFilter.SHOW_TEXT)`; for each text node,
   run the regex over `node.textContent`; for each match create a `Range`
   with `setStart(node, m.index)` / `setEnd(node, m.index + m[0].length)`.
4. Stop collecting when 1,500 ranges are reached (guard against pathological
   queries and WebKit's paint cost).
5. ALWAYS replace the registry entry — never delete it:
   `CSS.highlights.set('filter-match', new Highlight(...ranges))`, with an
   empty `Highlight` when nothing was collected.
6. Force the transcript repaint (see the forced-repaint decision above) so
   removed ranges actually disappear in WebKit.

**Why no other triggers.** The transcript DOM is static after load: text
nodes never change, so ranges stay valid across `<details>`
expand/collapse, theme toggles, and scrolling. Card visibility only changes
through the filter itself, which re-runs the refresh. Registered ranges in
collapsed details simply paint when the content becomes visible.

**Match granularity.** Matches are found within single text nodes. A query
spanning a formatting boundary (e.g. half plain, half bold text) still
filters the card correctly but that occurrence is not highlighted. Accepted
trade-off; cross-node matching is out of scope.

### CSS (`styles.css`)

`::highlight()` styling supports color and background only, and custom
properties are unreliable inside it, so the block uses literal colors,
mirroring the four existing theme scopes (default light, `@media
(prefers-color-scheme: dark)`, `:root[data-theme="dark"]`,
`:root[data-theme="light"]`):

- Light: `background-color: #fde68a; color: #292524;` (amber chip, dark text)
- Dark: `background-color: #92400e; color: #fef3c7;` (burnt amber, light text)

## Error handling

- No `CSS.highlights` support → feature-detect guard skips everything;
  filtering behaves exactly as today.
- Query cleared or shorter than 2 characters → registry entry replaced with
  an empty `Highlight` (never deleted; see the forced-repaint decision); no
  highlights linger.
- Zero surviving cards → step 3 finds no elements; an empty `Highlight` is
  registered; no visible effect.
- Cap reached → remaining occurrences unhighlighted; filtering itself is
  unaffected.

## Testing

No Go code changes, so no new Go tests; `gofmt`/`vet`/`go test -race ./...`
still gate the asset embedding. Behavior is verified manually with
Playwright against a served long report:

1. Type a query with ≥ 2 characters → visible occurrences highlighted in
   surviving cards; `CSS.highlights.has('filter-match')` is true.
2. Expand a collapsed tool card in a surviving turn → highlights visible
   inside the expanded content without re-typing.
3. Clear the filter → no highlights remain.
4. Single-character query → cards filter but nothing is highlighted.
5. Case-insensitivity: query in different case than the text still
   highlights.
6. Both themes show readable highlight colors.
7. Filter → scrub the timeline → clear filter: scroll anchoring still
   restores the focused card (no regression from this feature).

## Non-goals

- Cross-text-node match highlighting.
- Highlighting in the sidebar prompt list (the filter does not affect it).
- `<mark>`-based fallback for browsers without the Custom Highlight API.
- Multi-term/boolean queries (the filter is a single substring; highlighting
  mirrors it).
