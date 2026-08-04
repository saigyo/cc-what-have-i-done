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
    filter.addEventListener('input', function () {
      var q = filter.value.toLowerCase().trim();
      turns.forEach(function (t) {
        var hit = q === '' || (t.getAttribute('data-search') || '').indexOf(q) !== -1;
        t.classList.toggle('filtered', !hit);
      });
      syncDaySeps();
    });
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
          syncThumb();
        });
      }
    }

    var filterEl = document.getElementById('filter');
    var toggleAllBtn = document.getElementById('toggle-all');
    var toggleToolsBtn = document.getElementById('toggle-tools');

    if (filterEl) {
      filterEl.addEventListener('input', scheduleInvalidate);
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
})();
