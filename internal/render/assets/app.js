(function () {
  var filter = document.getElementById('filter');
  var turns = Array.prototype.slice.call(document.querySelectorAll('.turn[data-search]'));
  var daySeps = Array.prototype.slice.call(document.querySelectorAll('.day-sep'));
  // A day separator stays visible only while at least one turn before the
  // next separator survives the filter.
  function syncDaySeps() {
    daySeps.forEach(function (sep) {
      var visible = false;
      var el = sep.nextElementSibling;
      while (el && !el.classList.contains('day-sep')) {
        if (el.classList.contains('turn') && !el.classList.contains('filtered')) {
          visible = true;
          break;
        }
        el = el.nextElementSibling;
      }
      sep.classList.toggle('filtered', !visible);
    });
  }
  if (filter) {
    var applyFilter = function () {
      var q = filter.value.toLowerCase().trim();
      turns.forEach(function (t) {
        var hit = q === '' || (t.getAttribute('data-search') || '').indexOf(q) !== -1;
        t.classList.toggle('filtered', !hit);
      });
      syncDaySeps();
    };
    // Safari's native clear gestures (cancel button, Esc) fire only the
    // 'search' event, not 'input' — listen to both everywhere the filter
    // drives behavior.
    filter.addEventListener('input', applyFilter);
    filter.addEventListener('search', applyFilter);
  }

  var toggleAll = document.getElementById('toggle-all');
  var expanded = false;
  if (toggleAll) {
    toggleAll.addEventListener('click', function () {
      expanded = !expanded;
      document.querySelectorAll('details').forEach(function (d) { d.open = expanded; });
      toggleAll.textContent = expanded ? 'Collapse all' : 'Expand all';
    });
  }

  var toggleTools = document.getElementById('toggle-tools');
  if (toggleTools) {
    toggleTools.addEventListener('click', function () {
      var hidden = document.body.classList.toggle('hide-tools');
      toggleTools.textContent = hidden ? 'Show tool detail' : 'Hide tool detail';
    });
  }

  var toggleTheme = document.getElementById('toggle-theme');
  if (toggleTheme) {
    toggleTheme.addEventListener('click', function () {
      var cur = document.documentElement.getAttribute('data-theme');
      var next = cur === 'dark' ? 'light' : (cur === 'light' ? 'dark' : 'dark');
      document.documentElement.setAttribute('data-theme', next);
    });
  }

  // Links inside <summary> (agent transcript links) must navigate, not toggle
  // the surrounding <details>.
  document.addEventListener('click', (e) => {
    if (!(e.target instanceof Element)) return;
    const link = e.target.closest('a.agent-link');
    if (link) e.stopPropagation();
  }, true);

  // Transcript images render as capped thumbnails; a click toggles full size.
  document.addEventListener('click', function (e) {
    if (!(e.target instanceof Element)) return;
    var img = e.target.closest('img.turn-image');
    if (img) img.classList.toggle('expanded');
  });

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
    var cache = { height: -1, tops: [], labels: [], els: [] };
    var grabOffset = null;
    // Topmost in-view turn, remembered every settled frame so that when a DOM
    // mutation (filter change, expand/collapse) reflows the document, the view
    // can be re-anchored to the turn the reader was looking at.
    var anchor = null; // { el, delta: cached top - scrollY }

    function rebuildCache() {
      cache.height = doc.scrollHeight;
      cache.tops = [];
      cache.labels = [];
      cache.els = [];
      var scrollY = window.scrollY;
      document.querySelectorAll('.turn[data-time]').forEach(function (t) {
        if (t.classList.contains('filtered')) return;
        cache.tops.push(t.getBoundingClientRect().top + scrollY);
        cache.labels.push(t.getAttribute('data-time'));
        cache.els.push(t);
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

    // Index of the topmost turn in view: the last cached turn starting at or
    // above the line just below the sticky topbar (binary search), or the
    // first turn when none has started yet.
    function currentIndex() {
      if (!cache.tops.length) return -1;
      var threshold = window.scrollY + (topbar ? topbar.offsetHeight : 0) + 1;
      var lo = 0, hi = cache.tops.length - 1, best = 0;
      while (lo <= hi) {
        var mid = (lo + hi) >> 1;
        if (cache.tops[mid] <= threshold) { best = mid; lo = mid + 1; } else { hi = mid - 1; }
      }
      return best;
    }

    function currentLabel() {
      var i = currentIndex();
      return i < 0 ? '' : cache.labels[i];
    }

    function saveAnchor() {
      var i = currentIndex();
      // When nothing is visible (filter matched zero turns), keep the last
      // good anchor so clearing the filter can still restore the reader's place.
      if (i >= 0) anchor = { el: cache.els[i], delta: cache.tops[i] - window.scrollY };
    }

    // After a reflow, put the remembered turn back at its previous viewport
    // position. Skipped when the turn itself is filtered out (nothing sensible
    // to anchor to) — the browser's own clamping applies then.
    function restoreAnchor() {
      if (!anchor || anchor.el.classList.contains('filtered')) return;
      var top = anchor.el.getBoundingClientRect().top + window.scrollY;
      var y = Math.max(0, Math.min(maxScroll(), top - anchor.delta));
      if (Math.abs(y - window.scrollY) > 1) window.scrollTo(0, y);
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
      if (doc.scrollHeight !== cache.height) {
        // Height changed behind our back (e.g. the browser clamped the scroll
        // position after a filter shrank the page): rebuild, then re-anchor to
        // the turn from before the change instead of a now-arbitrary offset.
        rebuildCache();
        restoreAnchor();
      } else {
        saveAnchor();
      }
      syncThumb();
      showBubble();
    }

    window.addEventListener('scroll', function () {
      if (!rafPending) { rafPending = true; requestAnimationFrame(onFrame); }
    }, { passive: true });

    window.addEventListener('resize', function () {
      rebuildCache();
      restoreAnchor();
      syncThumb();
    });

    // Monitor DOM-mutating interactions (filter, expand/collapse, tool toggles)
    // to keep cache, thumb, and ticks in sync. Use requestAnimationFrame to defer
    // until DOM changes have been applied.
    var cacheInvalidatePending = false;
    function scheduleInvalidate() {
      if (!cacheInvalidatePending) {
        cacheInvalidatePending = true;
        requestAnimationFrame(function () {
          cacheInvalidatePending = false;
          rebuildCache();
          restoreAnchor();
          syncThumb();
        });
      }
    }

    var filterEl = document.getElementById('filter');
    var toggleAllBtn = document.getElementById('toggle-all');
    var toggleToolsBtn = document.getElementById('toggle-tools');

    if (filterEl) {
      filterEl.addEventListener('input', scheduleInvalidate);
      filterEl.addEventListener('search', scheduleInvalidate); // native clear gestures
    }
    if (toggleAllBtn) {
      toggleAllBtn.addEventListener('click', scheduleInvalidate);
    }
    if (toggleToolsBtn) {
      toggleToolsBtn.addEventListener('click', scheduleInvalidate);
    }

    // Listen for individual <details> toggles as well. The toggle event does not
    // bubble (bubbles: false), so use capture phase to intercept at the source.
    document.addEventListener('toggle', function (e) {
      if (e.target instanceof HTMLDetailsElement) {
        scheduleInvalidate();
      }
    }, true);

    function scrollToPointer(e) {
      var rect = track.getBoundingClientRect();
      var thumbH = thumb.offsetHeight;
      var off = grabOffset === null ? thumbH / 2 : grabOffset;
      var frac = (e.clientY - rect.top - off) / Math.max(1, rect.height - thumbH);
      frac = Math.max(0, Math.min(1, frac));
      window.scrollTo(0, frac * maxScroll());
    }

    timeline.addEventListener('pointerdown', function (e) {
      if (e.button !== 0) return;
      dragging = true;
      timeline.classList.add('dragging');
      timeline.setPointerCapture(e.pointerId);
      // Calculate grab offset if pointer is over the thumb; otherwise center-jump
      // behavior when clicking on the track.
      var thumbRect = thumb.getBoundingClientRect();
      if (e.clientY >= thumbRect.top && e.clientY <= thumbRect.bottom) {
        grabOffset = e.clientY - thumbRect.top;
      } else {
        grabOffset = null;
      }
      scrollToPointer(e);
      // A thumb grab preserves the scroll position, so no scroll event fires;
      // update the UI directly so the bubble appears as soon as the drag starts.
      syncThumb();
      showBubble();
      e.preventDefault();
    });
    timeline.addEventListener('pointermove', function (e) {
      if (dragging) scrollToPointer(e);
    });
    function endDrag() {
      if (!dragging) return;
      dragging = false;
      timeline.classList.remove('dragging');
      grabOffset = null;
      showBubble(); // restart the fade timer now that the drag has ended
    }
    timeline.addEventListener('pointerup', endDrag);
    timeline.addEventListener('pointercancel', endDrag);

    rebuildCache();
    syncThumb();
  }

  // --- Filter match highlighting ------------------------------------------
  // Paints every occurrence of the filter query in surviving cards via the
  // CSS Custom Highlight API (registry entry "filter-match") — no DOM
  // mutation, so ranges stay valid across <details> toggles and never
  // interact with the timeline's offset cache. Browsers without support
  // keep plain filtering.
  if (typeof CSS !== 'undefined' && CSS.highlights && filter) {
    var HIGHLIGHT_MIN = 2;
    var HIGHLIGHT_CAP = 5000;
    var highlightTimer = null;

    function refreshHighlights() {
      CSS.highlights.delete('filter-match');
      var q = filter.value.trim();
      if (q.length < HIGHLIGHT_MIN) return;
      // Regex matching keeps indices exact even for characters whose
      // lowercase form changes string length.
      var re = new RegExp(q.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi');
      var ranges = [];
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
      if (ranges.length) {
        CSS.highlights.set('filter-match', new Highlight(...ranges));
      }
    }

    var scheduleHighlights = function () {
      if (highlightTimer) clearTimeout(highlightTimer);
      highlightTimer = setTimeout(refreshHighlights, 150);
    };
    filter.addEventListener('input', scheduleHighlights);
    filter.addEventListener('search', scheduleHighlights); // native clear gestures
  }
})();
