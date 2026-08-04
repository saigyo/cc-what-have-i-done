# Filter Match Highlighting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** While the turn filter is active, highlight every occurrence of the query in surviving cards (including inside collapsed tool details) via the CSS Custom Highlight API.

**Architecture:** Pure client-side: one new self-contained section at the end of the existing IIFE in `app.js` (feature-detected, debounced, TreeWalker + regex over surviving cards, ranges registered as `CSS.highlights` entry `filter-match`) and one literal-color `::highlight` block in `styles.css` mirroring the four theme scopes. No Go, template, or markup changes.

**Tech Stack:** Vanilla JS (CSS Custom Highlight API, TreeWalker, Range), plain CSS. Go toolchain only as the embedding gate.

## Global Constraints

- No new dependencies; no Go or template changes.
- Gates on the task: `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...` (assets are embedded, so this rebuilds with the new JS/CSS).
- Highlight registry name: `filter-match`, styled via `::highlight(filter-match)`.
- Feature detection: `typeof CSS !== 'undefined' && CSS.highlights`; without support the section does nothing.
- Minimum query length for highlighting: 2 characters (after `trim()`). Debounce: 250 ms. Range cap: 1500. (Both retuned for WebKit paint cost during the WebKit hardening fixes; the original plan said 150 ms/5000.)
- WebKit hardening (added during interactive testing): the registry entry is always REPLACED via `set()` (never deleted); after each refresh `<main>` is promoted to a compositing layer for one frame (`transform: translateZ(0)`, prior inline transform restored on the next rAF) because WebKit does not repaint removed highlight ranges; all filter listeners register for both `input` and `search` (Safari-family native clear gestures fire only `search`).
- Highlight colors (literal, not custom properties): light `background-color: #fde68a; color: #292524;` — dark `background-color: #92400e; color: #fef3c7;` — mirrored across the four existing theme scopes (default, `@media (prefers-color-scheme: dark)`, `:root[data-theme="dark"]`, `:root[data-theme="light"]`).

---

### Task 1: Highlight section in app.js + ::highlight styles

**Files:**
- Modify: `internal/render/assets/app.js` (append inside the IIFE, immediately before the final `})();` at the file end, i.e. after the timeline section)
- Modify: `internal/render/assets/styles.css` (append at end)

**Interfaces:**
- Consumes: `#filter` input element (already used at app.js:2); `.filtered` class toggled by the existing filter handler at app.js:~22 (listener order guarantees that handler runs first because it registers first).
- Produces: nothing consumed elsewhere — final task.

- [ ] **Step 1: Append the highlight section to `app.js`**

Insert immediately before the closing `})();`:

```js
  // --- Filter match highlighting ------------------------------------------
  // Paints every occurrence of the filter query in surviving cards via the
  // CSS Custom Highlight API (registry entry "filter-match") — no DOM
  // mutation, so ranges stay valid across <details> toggles and never
  // interact with the timeline's offset cache. Browsers without support
  // keep plain filtering.
  if (typeof CSS !== 'undefined' && CSS.highlights && filter) {
    var HIGHLIGHT_MIN = 2;
    var HIGHLIGHT_CAP = 1500; // WebKit visibly freezes painting thousands of ranges
    var highlightTimer = null;
    var mainEl = document.querySelector('main');

    function refreshHighlights() {
      var q = filter.value.trim();
      var ranges = [];
      if (q.length >= HIGHLIGHT_MIN) {
        // Regex matching keeps indices exact even for characters whose
        // lowercase form changes string length.
        var re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
        var turnsToScan = document.querySelectorAll('.turn:not(.filtered)');
        outer:
        for (var i = 0; i < turnsToScan.length; i++) {
          var walker = document.createTreeWalker(turnsToScan[i], NodeFilter.SHOW_TEXT);
          var node;
          while ((node = walker.nextNode())) {
            var text = node.textContent;
            var m;
            re.lastIndex = 0;
            while ((m = re.exec(text))) {
              var r = document.createRange();
              r.setStart(node, m.index);
              r.setEnd(node, m.index + m[0].length);
              ranges.push(r);
              if (ranges.length >= HIGHLIGHT_CAP) break outer;
              if (m.index === re.lastIndex) re.lastIndex++; // zero-length guard
            }
          }
        }
      }
      CSS.highlights.set('filter-match', new Highlight(...ranges));
      // WebKit does not repaint regions whose ranges were removed from the
      // registry (neither on delete nor on replace) — stale highlights
      // linger until the text repaints for some other reason. Force one
      // repaint of the transcript by promoting <main> to its own
      // compositing layer for a single frame (no layout change, the fixed
      // timeline rail is a sibling and unaffected).
      if (mainEl) {
        var prevTransform = mainEl.style.transform;
        mainEl.style.transform = 'translateZ(0)';
        requestAnimationFrame(function () { mainEl.style.transform = prevTransform; });
      }
    }

    var scheduleHighlights = function () {
      if (highlightTimer) clearTimeout(highlightTimer);
      // 250 ms: long enough that fast typing skips intermediate prefixes
      // ("sh", "shi"), whose huge match counts are what WebKit chokes on.
      highlightTimer = setTimeout(refreshHighlights, 250);
    };
    filter.addEventListener('input', scheduleHighlights);
    filter.addEventListener('search', scheduleHighlights); // native clear gestures
  }
```

- [ ] **Step 2: Append the highlight styles to `styles.css`**

```css
/* Filter match highlighting (Custom Highlight API). ::highlight() cannot
   reliably use custom properties, hence literal colors per theme scope. */
::highlight(filter-match) { background-color: #fde68a; color: #292524; }
@media (prefers-color-scheme: dark) {
  ::highlight(filter-match) { background-color: #92400e; color: #fef3c7; }
}
:root[data-theme="dark"] ::highlight(filter-match) { background-color: #92400e; color: #fef3c7; }
:root[data-theme="light"] ::highlight(filter-match) { background-color: #fde68a; color: #292524; }
```

- [ ] **Step 3: Syntax-check the JS**

Run: `node --check internal/render/assets/app.js`
Expected: no output (valid syntax).

- [ ] **Step 4: Run the gates**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: gofmt prints nothing; vet clean; all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/render/assets/app.js internal/render/assets/styles.css
git commit -m "feat(render): highlight filter matches via CSS Custom Highlight API"
```

---

### Post-plan verification (controller, not a task)

Render a long report, serve it, verify with Playwright: ≥2-char query highlights visible occurrences (`CSS.highlights.has('filter-match')` true); a collapsed tool card in a surviving turn shows highlights when expanded without re-typing; clearing the filter removes all highlights; 1-char query filters but highlights nothing; case-insensitive matching; readable colors in both themes; filter → timeline scrub → clear-filter scroll anchoring unregressed.
