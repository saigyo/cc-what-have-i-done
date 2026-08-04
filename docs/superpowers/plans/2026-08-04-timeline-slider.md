# Timeline Slider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Floating timeline slider at the right viewport edge of session reports: draggable thumb mirroring scroll position, a date/time bubble while scrubbing or scrolling, and day-boundary tick marks.

**Architecture:** The Go renderer contributes a pre-formatted `data-time` attribute per turn card and a static rail skeleton in the shared template; all behavior is a self-contained section in `app.js`, all appearance in `styles.css`. One height-keyed offset cache in JS handles filtering, collapsing, and resizing through a single invalidation mechanism.

**Tech Stack:** Go 1.26 stdlib only (html/template, time), vanilla JS (Pointer Events), plain CSS. Tests with the standard `testing` package.

## Global Constraints

- Go 1.26, stdlib only — no new dependencies.
- Gates on every task: `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`.
- `data-time` value: Go layout `January 2 · 15:04` applied to the turn timestamp converted with `.Local()` (example: `July 29 · 14:03`). Empty/absent when the turn has no timestamp.
- Rail eligibility: shown only when `scrollHeight >= 3 * innerHeight`; also hidden by CSS below 760 px viewport width.
- Bubble fades 1000 ms after the last scroll/drag event.
- Thumb `min-height: 24px`. Rail carries `aria-hidden="true"`.
- Tests that construct local times must use the timezone-safe pattern: build the instant with `time.Local`, feed `.UTC()` where a real transcript would store UTC, assert on the `.Local()`-formatted value.

---

### Task 1: Renderer — `TimeBubble` field, `data-time` attribute, rail skeleton

**Files:**
- Modify: `internal/render/render.go` (turnView struct ~line 178, buildViewModel loop ~line 272)
- Modify: `internal/render/assets/report.html.tmpl` (article tag line 57, rail skeleton before the script include at the file end)
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: existing `turnView`, `buildViewModel`, `timeParts`, `dayTracker` — all already in `internal/render`.
- Produces: `turnView.TimeBubble string`; HTML contract for Task 2: every timestamped turn article carries `data-time="July 29 · 14:03"`, and every page contains `<div class="timeline hidden" aria-hidden="true">` wrapping `.timeline-track` > `.timeline-thumb`, plus a sibling `.timeline-bubble`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go`:

```go
func TestBuildViewModelTimeBubble(t *testing.T) {
	ts := time.Date(2026, 7, 29, 14, 3, 59, 0, time.Local)
	sess := model.Session{Turns: []model.Turn{
		{Kind: model.TurnUser, Timestamp: ts.UTC(), // stored as UTC, like real transcripts
			Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
		{Kind: model.TurnAssistant, // no timestamp
			Blocks: []model.Block{{Type: model.BlockText, Text: "hello"}}},
	}}
	d := buildViewModel(sess, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""), "")
	if d.Turns[0].TimeBubble != "July 29 · 14:03" {
		t.Errorf("TimeBubble = %q, want %q", d.Turns[0].TimeBubble, "July 29 · 14:03")
	}
	if d.Turns[1].TimeBubble != "" {
		t.Errorf("timestampless turn TimeBubble = %q, want empty", d.Turns[1].TimeBubble)
	}
}

func TestSiteRendersTimelineRailAndDataTime(t *testing.T) {
	ts := time.Date(2026, 7, 29, 14, 3, 59, 0, time.Local)
	sess := model.Session{
		StartedAt: ts,
		Turns: []model.Turn{
			{Kind: model.TurnUser, Timestamp: ts,
				Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
			{Kind: model.TurnAssistant, // no timestamp: must not carry data-time
				Blocks: []model.Block{{Type: model.BlockText, Text: "hello"}}},
		},
	}
	dir := t.TempDir()
	if err := Site(sess, dir, Options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `data-time="July 29 · 14:03"`) {
		t.Error("data-time attribute missing from timestamped turn")
	}
	if strings.Contains(out, `id="turn-1" data-time=`) {
		t.Error("timestampless turn must not carry data-time")
	}
	for _, want := range []string{
		`class="timeline hidden" aria-hidden="true"`,
		`class="timeline-track"`,
		`class="timeline-thumb"`,
		`class="timeline-bubble"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rail skeleton part %s missing from report", want)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render/ -run 'TimeBubble|TimelineRail' -v`
Expected: FAIL — `d.Turns[0].TimeBubble undefined` (compile error) or assertion failures.

- [ ] **Step 3: Add the `TimeBubble` field**

In `internal/render/render.go`, extend `turnView` (after the `DayHeader` field, ~line 189):

```go
	TimeBubble string // "July 29 · 14:03" timeline-bubble label; "" when no timestamp
```

- [ ] **Step 4: Set it in `buildViewModel`**

Replace the existing turn-day block (~lines 272–277):

```go
		tv.TimeLabel, tv.TimeTitle = timeParts(t.Timestamp)
		if !t.Timestamp.IsZero() {
			if lt := t.Timestamp.Local(); turnDays.newDay(lt) {
				tv.DayHeader = lt.Format("Monday, January 2, 2006")
			}
		}
```

with:

```go
		tv.TimeLabel, tv.TimeTitle = timeParts(t.Timestamp)
		if !t.Timestamp.IsZero() {
			lt := t.Timestamp.Local()
			tv.TimeBubble = lt.Format("January 2 · 15:04")
			if turnDays.newDay(lt) {
				tv.DayHeader = lt.Format("Monday, January 2, 2006")
			}
		}
```

- [ ] **Step 5: Emit the attribute in the template**

In `internal/render/assets/report.html.tmpl`, line 57, change the article opening tag from:

```html
    <article class="turn turn-{{ .Kind }}" id="turn-{{ .Index }}" data-search="{{ .SearchText }}">
```

to:

```html
    <article class="turn turn-{{ .Kind }}" id="turn-{{ .Index }}"{{ if .TimeBubble }} data-time="{{ .TimeBubble }}"{{ end }} data-search="{{ .SearchText }}">
```

- [ ] **Step 6: Add the rail skeleton**

In the same template, between `</main>` and the `<script src="{{ .Base }}assets/app.js"></script>` line at the end of the file, insert:

```html
<div class="timeline hidden" aria-hidden="true">
  <div class="timeline-track">
    <div class="timeline-thumb"></div>
  </div>
  <div class="timeline-bubble"></div>
</div>
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/render/ -run 'TimeBubble|TimelineRail' -v`
Expected: PASS

- [ ] **Step 8: Run the gates**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: gofmt prints nothing; vet clean; all tests pass.

- [ ] **Step 9: Commit**

```bash
git add internal/render/render.go internal/render/assets/report.html.tmpl internal/render/render_test.go
git commit -m "feat(render): data-time attribute and timeline rail skeleton"
```

---

### Task 2: Timeline behavior (app.js) and styling (styles.css)

**Files:**
- Modify: `internal/render/assets/app.js` (append inside the existing IIFE, before the final `})();`)
- Modify: `internal/render/assets/styles.css` (append at end)

**Interfaces:**
- Consumes: the Task 1 HTML contract — `.timeline` (with `hidden` class initially) containing `.timeline-track` > `.timeline-thumb` and `.timeline-bubble`; turn articles with `data-time="July 29 · 14:03"`; existing `.filtered` class toggled by the filter; existing `.day-sep` elements whose text content is the weekday-form day header; sticky `.topbar`.
- Produces: nothing consumed later — this is the final task.

- [ ] **Step 1: Append the timeline section to `app.js`**

Insert before the closing `})();` of the IIFE:

```js
  // --- Floating timeline slider (right edge) ------------------------------
  // Scroll-proportional rail: thumb mirrors scroll position, dragging scrolls
  // the page, a bubble shows the date/time of the topmost turn in view, and
  // ticks mark day boundaries. Hidden entirely on short documents.
  var timeline = document.querySelector('.timeline');
  if (timeline) {
    var track = timeline.querySelector('.timeline-track');
    var thumb = timeline.querySelector('.timeline-thumb');
    var bubble = timeline.querySelector('.timeline-bubble');
    var topbar = document.querySelector('.topbar');
    var doc = document.documentElement;
    var THUMB_MIN = 24;
    var dragging = false;
    var fadeTimer = null;
    var rafPending = false;

    // Offset cache keyed by the scrollHeight it was built at. Anything that
    // changes document height (filter, expand/collapse, tool toggles,
    // resizes) is caught by comparing live scrollHeight against cache.height.
    var cache = { height: -1, tops: [], labels: [] };

    function rebuildCache() {
      cache.height = doc.scrollHeight;
      cache.tops = [];
      cache.labels = [];
      var scrollY = window.scrollY;
      document.querySelectorAll('.turn[data-time]').forEach(function (t) {
        if (t.classList.contains('filtered')) return;
        cache.tops.push(t.getBoundingClientRect().top + scrollY);
        cache.labels.push(t.getAttribute('data-time'));
      });
      rebuildTicks(scrollY);
      timeline.classList.toggle('hidden', doc.scrollHeight < 3 * window.innerHeight);
    }

    function rebuildTicks(scrollY) {
      track.querySelectorAll('.timeline-tick').forEach(function (el) { el.remove(); });
      document.querySelectorAll('.day-sep').forEach(function (sep) {
        if (sep.classList.contains('filtered')) return;
        var frac = (sep.getBoundingClientRect().top + scrollY) / cache.height;
        var tick = document.createElement('div');
        tick.className = 'timeline-tick';
        tick.style.top = (frac * 100) + '%';
        tick.title = sep.textContent.trim();
        track.appendChild(tick);
      });
    }

    function maxScroll() {
      return Math.max(1, doc.scrollHeight - window.innerHeight);
    }

    function syncThumb() {
      var trackH = track.clientHeight;
      var thumbH = Math.max(THUMB_MIN, trackH * window.innerHeight / doc.scrollHeight);
      var top = (window.scrollY / maxScroll()) * (trackH - thumbH);
      thumb.style.height = thumbH + 'px';
      thumb.style.top = top + 'px';
      bubble.style.top = (top + thumbH / 2) + 'px';
    }

    // Label of the topmost turn in view: the last cached turn starting at or
    // above the line just below the sticky topbar (binary search), or the
    // first turn when none has started yet.
    function currentLabel() {
      if (!cache.labels.length) return '';
      var threshold = window.scrollY + (topbar ? topbar.offsetHeight : 0) + 1;
      var lo = 0, hi = cache.tops.length - 1, best = 0;
      while (lo <= hi) {
        var mid = (lo + hi) >> 1;
        if (cache.tops[mid] <= threshold) { best = mid; lo = mid + 1; } else { hi = mid - 1; }
      }
      return cache.labels[best];
    }

    function showBubble() {
      var label = currentLabel();
      if (!label) { bubble.classList.remove('visible'); return; }
      bubble.textContent = label;
      bubble.classList.add('visible');
      if (fadeTimer) clearTimeout(fadeTimer);
      fadeTimer = setTimeout(function () {
        if (!dragging) bubble.classList.remove('visible');
      }, 1000);
    }

    function onFrame() {
      rafPending = false;
      if (doc.scrollHeight !== cache.height) rebuildCache();
      syncThumb();
      showBubble();
    }

    window.addEventListener('scroll', function () {
      if (!rafPending) { rafPending = true; requestAnimationFrame(onFrame); }
    }, { passive: true });

    window.addEventListener('resize', function () {
      rebuildCache();
      syncThumb();
    });

    function scrollToPointer(e) {
      var rect = track.getBoundingClientRect();
      var thumbH = thumb.offsetHeight;
      var frac = (e.clientY - rect.top - thumbH / 2) / Math.max(1, rect.height - thumbH);
      frac = Math.max(0, Math.min(1, frac));
      window.scrollTo(0, frac * maxScroll());
    }

    timeline.addEventListener('pointerdown', function (e) {
      if (e.button !== 0) return;
      dragging = true;
      timeline.classList.add('dragging');
      timeline.setPointerCapture(e.pointerId);
      scrollToPointer(e);
      e.preventDefault();
    });
    timeline.addEventListener('pointermove', function (e) {
      if (dragging) scrollToPointer(e);
    });
    function endDrag() {
      if (!dragging) return;
      dragging = false;
      timeline.classList.remove('dragging');
      showBubble(); // restart the fade timer now that the drag has ended
    }
    timeline.addEventListener('pointerup', endDrag);
    timeline.addEventListener('pointercancel', endDrag);

    rebuildCache();
    syncThumb();
  }
```

- [ ] **Step 2: Append the timeline styles to `styles.css`**

```css
/* Floating timeline slider */
.timeline { position: fixed; right: 6px; top: 4.5rem; bottom: 1rem; width: 8px; z-index: 20; opacity: .55; transition: opacity .15s; touch-action: none; }
.timeline:hover, .timeline.dragging { opacity: 1; }
.timeline.hidden { display: none; }
.timeline-track { position: relative; height: 100%; width: 100%; background: var(--border); border-radius: 4px; }
.timeline-thumb { position: absolute; left: 0; width: 100%; min-height: 24px; background: var(--muted); border-radius: 4px; cursor: grab; }
.timeline.dragging .timeline-thumb { cursor: grabbing; }
.timeline-tick { position: absolute; left: 0; width: 100%; height: 1px; background: var(--muted); opacity: .6; pointer-events: none; }
.timeline-bubble { position: absolute; right: 14px; transform: translateY(-50%); background: var(--surface); border: 1px solid var(--border); border-radius: 6px; padding: .15rem .5rem; font-size: .8rem; white-space: nowrap; opacity: 0; transition: opacity .2s; pointer-events: none; }
.timeline-bubble.visible { opacity: 1; }
@media (max-width: 760px) { .timeline { display: none; } }
```

- [ ] **Step 3: Run the gates**

Run: `gofmt -l . && go vet ./... && go test -race ./...`
Expected: gofmt prints nothing; vet clean; all tests pass (assets are embedded, so this rebuilds and re-renders with the new JS/CSS).

- [ ] **Step 4: Commit**

```bash
git add internal/render/assets/app.js internal/render/assets/styles.css
git commit -m "feat(render): floating timeline slider behavior and styles"
```

---

### Post-plan verification (controller, not a task)

After both tasks: regenerate a long demo report locally, serve it over `python3 -m http.server`, and verify with Playwright — thumb drag scrolls, bubble shows `July 29 · 14:03`-style labels and fades ~1 s after scrolling stops, day ticks align with day separators, rail hidden on short reports and below 760 px, both themes look right, filter interaction rebuilds ticks. The committed demo report under `docs/session-report/` is NOT regenerated in this feature branch.
