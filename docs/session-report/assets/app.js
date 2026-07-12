(function () {
  var filter = document.getElementById('filter');
  var turns = Array.prototype.slice.call(document.querySelectorAll('.turn[data-search]'));
  if (filter) {
    filter.addEventListener('input', function () {
      var q = filter.value.toLowerCase().trim();
      turns.forEach(function (t) {
        var hit = q === '' || (t.getAttribute('data-search') || '').indexOf(q) !== -1;
        t.classList.toggle('filtered', !hit);
      });
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
})();
