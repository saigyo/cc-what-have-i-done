# Tool-Card Image Badge — Design

**Status:** approved (2026-07-20)

**Goal:** Collapsed tool cards indicate in their header when they contain
images, so readers don't have to expand cards hunting for screenshots.

## Motivation

Since the report-images feature, tool-result images render inside the
collapsed `<details class="tool">` body. A session with a dozen Playwright
screenshots gives no hint which cards hold them. A small header badge fixes
that. (User-pasted images need no indicator — they sit directly in the turn
body, always visible.)

## Decisions

- **Scope:** a card is badged when its collapsed content *on this page*
  contains images: its own `Result.Images` plus images anywhere inside its
  nested sidechain turns (`tc.Subagents`, recursively — sidechain turns can
  contain tool calls with their own images). Linked agent sessions on
  separate pages do NOT badge the Agent card; those pages badge their own
  cards.
- **Form:** `📷` for one image, `📷 N` for N > 1 — small, muted, same
  visual weight as the per-turn usage badge.
- **`--no-images` mode:** the badge still shows (it marks "this card
  involved images", which is extra useful when only placeholders are
  inside). One code path.
- **Placement:** in `<summary class="tool-head">`, after the tool-summary
  span, before the transcript link.

## Design

### Counting (`internal/render/images.go`)

- New `forEachToolCallImage(tc *model.ToolCall, fn func(model.Image))`:
  calls fn for the tool call's `Result.Images` and, recursively, for every
  image inside `tc.Subagents` turns (`BlockImage` blocks and nested tool
  calls).
- `forEachImage` is refactored to use it (turn walk stays; the per-tool
  part moves into the new helper). No behavior change — asset writing
  output is identical.
- New `toolImageCount(tc *model.ToolCall) int`, implemented via
  `forEachToolCallImage` with a counter.

### Markup (`internal/render/render.go`, in `renderTool`'s summary)

After the tool-summary span, before the agent-link:

```html
<span class="image-badge"><span class="image-badge-icon">📷</span></span>      <!-- count == 1 -->
<span class="image-badge"><span class="image-badge-icon">📷</span> 3</span>    <!-- count > 1 -->
```

Emitted only when `toolImageCount(tc) > 0`, independent of `NoImages`.
The count is `strconv.Itoa` output; the literal markup needs no escaping
but stays inside the existing manually-built, escape-disciplined summary.

### CSS (`internal/render/assets/styles.css`)

```css
.image-badge { color: var(--muted); font-size: .75rem; font-weight: 400; white-space: nowrap; flex-shrink: 0; }
/* Default (Chrome/Blink, Firefox/Gecko): small lift, slightly larger glyph. */
.image-badge-icon { position: relative; top: -0.04em; font-size: 0.85rem; }
/* WebKit only (Safari, DuckDuckGo on macOS) — -webkit-hyphens exists in no
   other engine: bigger lift, stock size. */
@supports (-webkit-hyphens: none) {
  .image-badge-icon { top: -0.09em; font-size: 0.75rem; }
}
```

The icon span lifts only the emoji toward the optical text baseline; count
digits stay on the true baseline. The `@supports` feature query targets the
rendering *engine* (the axis emoji metrics actually vary on), not the
browser brand. It is a compatibility hack by nature: if Blink ever adopted
`-webkit-hyphens`, Chrome would take the WebKit branch — failure mode is a
0.05em cosmetic shift, nothing breaks.

Related hardening in the same flex row: `.agent-link` gains
`flex-shrink: 0` so long summaries can never squeeze it (or the badge) out
of view — the summary span is the only shrinkable element
(`overflow: hidden` gives it a zero flex minimum), so it ellipsizes exactly
as today while trailing header items stay pinned and fully visible.

## Out of scope

- No badge on Agent cards for images that live on linked agent pages.
- No badges for thinking blocks (text-only) or user-pasted images (visible,
  not collapsed).
- No changes to search text (`[image]` already indexes image turns).

## Testing

- Unit: `toolImageCount` — 0 for nil/imageless result; N for result images;
  counts images in nested sidechain turns (including a tool call inside a
  sidechain turn); sums both sources.
- Render: card with 2 result images shows `📷</span> 2` in its summary;
  single image shows icon-only badge (no count, no trailing space); zero
  images → no `image-badge` in that card; badge present with
  `Options{NoImages: true}`; badge precedes the agent-link for a Task card.
- Regression: existing image-asset tests (dedup, e2e) stay green across the
  `forEachImage` refactor.
