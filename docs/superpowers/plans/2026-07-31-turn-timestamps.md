# Turn Time Labels & Day Segmentation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every turn card carries a local-time label next to its role label ("Claude · 14:03", "You · 14:03") with a full date/time hover title, and the transcript is segmented by calendar days with centered day headers.

**Architecture:** Timestamps already exist on `model.Turn.Timestamp` (UTC). A pure `timeParts` helper formats the card label and hover title in local time; `buildViewModel` sets them on every `turnView` and computes `DayHeader` on the first timestamped turn of each local calendar day. The template renders the label inside `.turn-role` (native `title` tooltip) and a `.day-sep` div before day-starting turns. CSS styles the separator; a small `app.js` addition hides day separators whose turns are all filtered out.

**Tech Stack:** Go stdlib only (`time`); plain `testing`; vanilla CSS/JS in the embedded assets.

## Global Constraints

- Timezone: convert with `.Local()` at render time (render machine's timezone). No client-side time formatting.
- Formats, exactly: card label `15:04`; hover title `January 2, 2006 at 15:04:05`; day header `Monday, January 2, 2006` (Go reference-time layouts).
- The label appears on every turn kind — user, assistant, and agent-result alike.
- Zero timestamps: no label, no tooltip, no day header, and they do not participate in day-change detection.
- Main page and subagent pages share the code path — no page-specific logic.
- No changes to `internal/transcript`, `internal/model`, or turn search text.
- No new go.mod dependencies. gofmt-clean, `go vet ./...` clean, tests pass with `-race`.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

---

### Task 1: timeParts helper, view-model fields, template

**Files:**
- Modify: `internal/render/format.go` (add `timeParts` + `time` import)
- Modify: `internal/render/render.go` (`turnView` at :166-175, `buildViewModel` at :197-239, `time` import)
- Modify: `internal/render/assets/report.html.tmpl` (turn loop)
- Test: `internal/render/format_test.go`, `internal/render/render_test.go`

**Interfaces:**
- Consumes: `model.Turn.Timestamp time.Time` (already parsed, UTC; zero when missing); `viewData.StartedAt` (session meta line).
- Produces: `timeParts(ts time.Time) (label, title string)`; `turnView.TimeLabel`, `turnView.TimeTitle`, `turnView.DayHeader` (all `string`); template classes `turn-time` and `day-sep` that Task 2 styles.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/format_test.go` (add `"time"` to its imports):

```go
func TestTimeParts(t *testing.T) {
	if l, ti := timeParts(time.Time{}); l != "" || ti != "" {
		t.Fatalf("zero time: got %q, %q; want empty strings", l, ti)
	}
	// Construct the instant in the local zone so expectations are concrete
	// and the test passes in any timezone.
	ts := time.Date(2026, 7, 29, 14, 3, 59, 0, time.Local)
	l, ti := timeParts(ts)
	if l != "14:03" {
		t.Errorf("label = %q, want %q", l, "14:03")
	}
	if ti != "July 29, 2026 at 14:03:59" {
		t.Errorf("title = %q, want %q", ti, "July 29, 2026 at 14:03:59")
	}
	// The same instant expressed in UTC must convert back to local.
	l2, ti2 := timeParts(ts.UTC())
	if l2 != l || ti2 != ti {
		t.Errorf("UTC input: got %q, %q; want %q, %q", l2, ti2, l, ti)
	}
}
```

Append to `internal/render/render_test.go` (its imports already include `time` — verify, add if missing):

```go
func TestBuildViewModelTimeLabelsAndDayHeaders(t *testing.T) {
	day1 := time.Date(2026, 7, 29, 9, 0, 0, 0, time.Local)
	day2 := day1.AddDate(0, 0, 1)
	sess := model.Session{Turns: []model.Turn{
		{Kind: model.TurnUser, Timestamp: day1},
		{Kind: model.TurnAssistant}, // no timestamp
		{Kind: model.TurnAssistant, Timestamp: day1.Add(time.Hour)},
		{Kind: model.TurnUser, Timestamp: day2},
	}}
	d := buildViewModel(sess, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""))
	turns := d.Turns
	if len(turns) != 4 {
		t.Fatalf("got %d turns, want 4", len(turns))
	}
	// First timestamped turn opens day one — user turns get labels too.
	if turns[0].DayHeader != day1.Format("Monday, January 2, 2006") {
		t.Errorf("turn 0 DayHeader = %q", turns[0].DayHeader)
	}
	if turns[0].TimeLabel != day1.Format("15:04") {
		t.Errorf("turn 0 TimeLabel = %q", turns[0].TimeLabel)
	}
	// Zero timestamp: no label, no header, does not reset day detection.
	if turns[1].TimeLabel != "" || turns[1].TimeTitle != "" || turns[1].DayHeader != "" {
		t.Errorf("turn 1 (no timestamp) got %q, %q, %q; want all empty",
			turns[1].TimeLabel, turns[1].TimeTitle, turns[1].DayHeader)
	}
	// Same day: label but no header.
	if turns[2].DayHeader != "" {
		t.Errorf("turn 2 DayHeader = %q, want empty (same day)", turns[2].DayHeader)
	}
	if turns[2].TimeLabel == "" {
		t.Error("turn 2 TimeLabel empty, want set")
	}
	// New day: fresh header.
	if turns[3].DayHeader != day2.Format("Monday, January 2, 2006") {
		t.Errorf("turn 3 DayHeader = %q", turns[3].DayHeader)
	}
}

func TestSiteRendersTimeLabelsAndDaySeparator(t *testing.T) {
	ts := time.Date(2026, 7, 29, 14, 3, 59, 0, time.Local)
	sess := model.Session{
		StartedAt: ts,
		Turns: []model.Turn{
			{Kind: model.TurnUser, Timestamp: ts,
				Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
			{Kind: model.TurnAssistant, Timestamp: ts.Add(time.Minute),
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
	if !strings.Contains(out, `class="day-sep"`) {
		t.Error("day separator missing from report")
	}
	if !strings.Contains(out, `class="turn-time" title="July 29, 2026 at 14:03:59"`) {
		t.Error("turn-time span with full hover title missing from report")
	}
	if !strings.Contains(out, "· 14:03") {
		t.Error("time label missing from report")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestTimeParts|TestBuildViewModelTimeLabelsAndDayHeaders|TestSiteRendersTimeLabelsAndDaySeparator'`
Expected: FAIL to build — `undefined: timeParts` (and undefined `turnView` fields once that resolves).

- [ ] **Step 3: Implement**

1. In `internal/render/format.go`: add `"time"` to the imports and append:

```go
// timeParts formats a turn timestamp for display: the short card label and
// the full hover title, in local time. A zero time yields "", "".
func timeParts(ts time.Time) (label, title string) {
	if ts.IsZero() {
		return "", ""
	}
	lt := ts.Local()
	return lt.Format("15:04"), lt.Format("January 2, 2006 at 15:04:05")
}
```

2. In `internal/render/render.go`: add `"time"` to the imports. Extend `turnView`:

```go
type turnView struct {
	Index      int
	Kind       string
	RoleLabel  string
	Status     string // agent-result status chip, e.g. "completed"
	SearchText string
	Body       template.HTML
	Badge      string // per-turn usage badge, e.g. "12k tok · ~$0.18"
	AgentHref  string // link to the agent's transcript page, when one exists
	TimeLabel  string // "15:04" local-time card label; "" when no timestamp
	TimeTitle  string // hover title, e.g. "July 29, 2026 at 14:03:59"
	DayHeader  string // "Wednesday, July 29, 2026" on each day's first turn
}
```

3. In `buildViewModel`, change the `StartedAt` line to convert to local time:

```go
	if !s.StartedAt.IsZero() {
		d.StartedAt = s.StartedAt.Local().Format("2006-01-02 15:04")
	}
```

4. In the `for i, t := range s.Turns` loop, after the `tv := turnView{…}` literal and before the `if t.Kind == model.TurnAgentResult` block, add time fields and day detection (declare the trackers just above the loop):

```go
	var lastY, lastD int
	var lastM time.Month
	haveDay := false
	for i, t := range s.Turns {
		// … existing plain/prompts code and tv := turnView{…} literal …
		tv.TimeLabel, tv.TimeTitle = timeParts(t.Timestamp)
		if !t.Timestamp.IsZero() {
			lt := t.Timestamp.Local()
			y, m, dd := lt.Date()
			if !haveDay || y != lastY || m != lastM || dd != lastD {
				tv.DayHeader = lt.Format("Monday, January 2, 2006")
				lastY, lastM, lastD = y, m, dd
				haveDay = true
			}
		}
```

5. In `internal/render/assets/report.html.tmpl`, replace the turn loop's opening:

```html
    {{ range .Turns }}
    <article class="turn turn-{{ .Kind }}" id="turn-{{ .Index }}" data-search="{{ .SearchText }}">
      <div class="turn-role">{{ .RoleLabel }}{{ if .Status }}<span class="agent-status">{{ .Status }}</span>{{ end }}{{ if .AgentHref }}<a class="agent-link" href="{{ .AgentHref }}">transcript ↗</a>{{ end }}{{ if .Badge }}<span class="usage-badge">{{ .Badge }}</span>{{ end }}</div>
```

with:

```html
    {{ range .Turns }}
    {{ if .DayHeader }}<div class="day-sep"><span>{{ .DayHeader }}</span></div>{{ end }}
    <article class="turn turn-{{ .Kind }}" id="turn-{{ .Index }}" data-search="{{ .SearchText }}">
      <div class="turn-role">{{ .RoleLabel }}{{ if .TimeLabel }}<span class="turn-time" title="{{ .TimeTitle }}">· {{ .TimeLabel }}</span>{{ end }}{{ if .Status }}<span class="agent-status">{{ .Status }}</span>{{ end }}{{ if .AgentHref }}<a class="agent-link" href="{{ .AgentHref }}">transcript ↗</a>{{ end }}{{ if .Badge }}<span class="usage-badge">{{ .Badge }}</span>{{ end }}</div>
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/render/ -race`
Expected: PASS (new tests plus all existing render tests).

- [ ] **Step 5: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/render/format.go internal/render/format_test.go internal/render/render.go internal/render/render_test.go internal/render/assets/report.html.tmpl
git commit -m "feat(render): time labels on turn cards and day segmentation"
```

---

### Task 2: Styling and filter-aware day separators

**Files:**
- Modify: `internal/render/assets/styles.css`
- Modify: `internal/render/assets/app.js` (filter handler at the top of the IIFE)

**Interfaces:**
- Consumes: `.day-sep` / `.turn-time` markup from Task 1; the existing filter handler that toggles `.filtered` on `.turn` elements; `.turn.filtered { display: none; }` at styles.css:98.
- Produces: styled separator + label; `.day-sep.filtered` hiding rule; separator visibility synced to the filter.

- [ ] **Step 1: Add the CSS**

In `internal/render/assets/styles.css`, directly after the `.turn-user .turn-role` rule (line 49), insert:

```css
.turn-time { margin-left: .35rem; cursor: default; }
.day-sep { display: flex; align-items: center; gap: .75rem; color: var(--muted); font-size: .85rem; margin: 1.5rem 0 1rem; }
.day-sep::before, .day-sep::after { content: ""; flex: 1; border-top: 1px solid var(--border); }
.day-sep.filtered { display: none; }
```

(`.turn-time` sits inside `.turn-role`, so it inherits the muted color and small size; digits are unaffected by the uppercase transform.)

- [ ] **Step 2: Sync separators with the filter**

In `internal/render/assets/app.js`, the filter block currently reads:

```js
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
```

Replace it with:

```js
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
```

- [ ] **Step 3: Verify by rendering a real report**

Run:

```bash
go run ./cmd/ccwhid --help >/dev/null && \
go test ./internal/render/ -race
```

Then render any local session transcript to a temp dir and open it if available; otherwise rely on the render tests (assets are embedded, so `go test` re-embeds the edited CSS/JS) plus a manual spot-check:

```bash
grep -n "day-sep" internal/render/assets/styles.css internal/render/assets/app.js
```

Expected: tests PASS; both files contain the new rules/logic.

- [ ] **Step 4: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/assets/styles.css internal/render/assets/app.js
git commit -m "feat(render): style day separators and hide them when filtered out"
```
