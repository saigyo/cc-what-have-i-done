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
})();
