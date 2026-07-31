# Sidebar Prompt Times & Date Dividers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sidebar prompt entries show a right-aligned local-time label with a full date/time hover title, and compact date dividers segment the prompt list by calendar day.

**Architecture:** `promptRef` gains `TimeLabel`/`TimeTitle`/`DayHeader`/`DayTitle`, computed in the existing `buildViewModel` loop from the user turn's `Timestamp`, reusing `timeParts`. Day detection is extracted into a small `dayTracker` type used by two independent instances — one for turn cards (refactoring the existing inline trackers), one for prompts. The sidebar template renders divider `<li>`s (opted out of the CSS counter) and a two-span flex link.

**Tech Stack:** Go stdlib only (`time`); plain `testing`; vanilla CSS in the embedded assets. No JS changes.

## Global Constraints

- Time formats, exactly (Go reference-time layouts): prompt time label `15:04`; time hover title `January 2, 2006 at 15:04:05`; divider text `January 2, 2006`; divider hover title `Monday, January 2, 2006`.
- Timezone: `.Local()` conversion at render time; no client-side formatting.
- Sidebar day detection runs over prompts only, independent of the turn-card tracker: a day with turns but no prompts gets no sidebar divider, and a prompt on a day already seen by turn cards still gets its divider.
- Zero-timestamp prompts: no time span, no divider, no participation in day-change detection.
- Prompt numbering unchanged: the global CSS counter still counts prompt entries only (divider items use `counter-increment: none`).
- Main pane's day separators and turn cards: behavior unchanged (the `dayTracker` refactor must be behavior-preserving; existing tests must pass unmodified).
- No changes to `internal/transcript`, `internal/model`, or `app.js`. No new go.mod dependencies. gofmt-clean, `go vet ./...` clean, tests pass with `-race`.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

---

### Task 1: dayTracker, promptRef fields, template, CSS

**Files:**
- Modify: `internal/render/render.go` (`promptRef` at :162-165, `buildViewModel` loop at :221-254)
- Modify: `internal/render/assets/report.html.tmpl` (sidebar prompt loop)
- Modify: `internal/render/assets/styles.css` (`.prompt-list` block, ~lines 40-44)
- Test: `internal/render/render_test.go`

**Interfaces:**
- Consumes: `timeParts(ts time.Time) (label, title string)` from `internal/render/format.go` (existing); `model.Turn.Timestamp`.
- Produces: `dayTracker` struct with method `newDay(lt time.Time) bool` (package render); `promptRef.TimeLabel`, `.TimeTitle`, `.DayHeader`, `.DayTitle` (all `string`); template classes `prompt-day`, `prompt-preview`, `prompt-time`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/render_test.go`:

```go
func TestBuildViewModelPromptTimesAndDayDividers(t *testing.T) {
	day1 := time.Date(2026, 7, 29, 9, 0, 0, 0, time.Local)
	day2 := day1.AddDate(0, 0, 1)
	sess := model.Session{Turns: []model.Turn{
		{Kind: model.TurnUser, Timestamp: day1},
		// An assistant turn reaches day2 first: the turn-card tracker sees
		// day2 here, but the prompt on day2 below must still get a divider —
		// the two trackers are independent.
		{Kind: model.TurnAssistant, Timestamp: day2},
		{Kind: model.TurnUser}, // no timestamp
		{Kind: model.TurnUser, Timestamp: day2.Add(time.Hour)},
	}}
	d := buildViewModel(sess, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""))
	if len(d.Prompts) != 3 {
		t.Fatalf("got %d prompts, want 3", len(d.Prompts))
	}
	p := d.Prompts
	if p[0].DayHeader != day1.Format("January 2, 2006") {
		t.Errorf("prompt 0 DayHeader = %q", p[0].DayHeader)
	}
	if p[0].DayTitle != day1.Format("Monday, January 2, 2006") {
		t.Errorf("prompt 0 DayTitle = %q", p[0].DayTitle)
	}
	if p[0].TimeLabel != day1.Format("15:04") {
		t.Errorf("prompt 0 TimeLabel = %q", p[0].TimeLabel)
	}
	if p[0].TimeTitle != day1.Format("January 2, 2006 at 15:04:05") {
		t.Errorf("prompt 0 TimeTitle = %q", p[0].TimeTitle)
	}
	// Zero timestamp: none of the four fields, and no divider reset.
	if p[1].TimeLabel != "" || p[1].TimeTitle != "" || p[1].DayHeader != "" || p[1].DayTitle != "" {
		t.Errorf("prompt 1 (no timestamp) got %q, %q, %q, %q; want all empty",
			p[1].TimeLabel, p[1].TimeTitle, p[1].DayHeader, p[1].DayTitle)
	}
	// New prompt day gets a divider even though a turn card already saw day2.
	if p[2].DayHeader != day2.Format("January 2, 2006") {
		t.Errorf("prompt 2 DayHeader = %q, want day2 divider (independent tracker)", p[2].DayHeader)
	}
	// Turn cards are unaffected: the assistant turn still owns day2's separator.
	if d.Turns[1].DayHeader != day2.Format("Monday, January 2, 2006") {
		t.Errorf("turn 1 DayHeader = %q, want day2 header", d.Turns[1].DayHeader)
	}
	if d.Turns[3].DayHeader != "" {
		t.Errorf("turn 3 DayHeader = %q, want empty (day2 already seen by turns)", d.Turns[3].DayHeader)
	}
}

func TestSiteRendersPromptTimesAndDividers(t *testing.T) {
	ts := time.Date(2026, 7, 29, 14, 3, 59, 0, time.Local)
	sess := model.Session{
		StartedAt: ts,
		Turns: []model.Turn{
			{Kind: model.TurnUser, Timestamp: ts,
				Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}},
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
	if !strings.Contains(out, `<li class="prompt-day" title="`+ts.Format("Monday, January 2, 2006")+`">`+ts.Format("January 2, 2006")+`</li>`) {
		t.Error("prompt-day divider with weekday hover title missing from sidebar")
	}
	if !strings.Contains(out, `<span class="prompt-time" title="July 29, 2026 at 14:03:59">14:03</span>`) {
		t.Error("prompt-time span with hover title missing from sidebar")
	}
}
```

(The divider assertion builds its expectation from the same fixed local-time
instant, so it stays correct in any timezone; the time-title literal is safe
because the instant is constructed in `time.Local`.)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestBuildViewModelPromptTimesAndDayDividers|TestSiteRendersPromptTimesAndDividers'`
Expected: FAIL to build — `p[0].DayHeader undefined` (promptRef has no such field yet).

- [ ] **Step 3: Implement**

1. In `internal/render/render.go`, extend `promptRef`:

```go
type promptRef struct {
	Index     int
	Preview   string
	TimeLabel string // "15:04" local-time label; "" when no timestamp
	TimeTitle string // hover title, e.g. "July 29, 2026 at 14:03:59"
	DayHeader string // "July 29, 2026" on each day's first prompt
	DayTitle  string // "Wednesday, July 29, 2026" divider hover title
}
```

2. Add the `dayTracker` type directly above `buildViewModel`:

```go
// dayTracker reports whether a local time falls on a new calendar day
// relative to the last one it saw. The zero value has seen no day yet.
type dayTracker struct {
	year    int
	month   time.Month
	day     int
	haveDay bool
}

// newDay records lt's date and reports whether it starts a new day.
func (dt *dayTracker) newDay(lt time.Time) bool {
	y, m, d := lt.Date()
	if dt.haveDay && y == dt.year && m == dt.month && d == dt.day {
		return false
	}
	dt.year, dt.month, dt.day, dt.haveDay = y, m, d, true
	return true
}
```

3. In `buildViewModel`, replace the inline trackers and the loop's day/prompt handling. Delete these lines above the loop:

```go
	var lastY, lastD int
	var lastM time.Month
	haveDay := false
```

replacing them with:

```go
	var turnDays, promptDays dayTracker
```

Replace the prompt append inside the loop:

```go
		if t.Kind == model.TurnUser {
			d.Prompts = append(d.Prompts, promptRef{Index: i, Preview: preview(plain, 60)})
		}
```

with:

```go
		if t.Kind == model.TurnUser {
			p := promptRef{Index: i, Preview: preview(plain, 60)}
			p.TimeLabel, p.TimeTitle = timeParts(t.Timestamp)
			if !t.Timestamp.IsZero() {
				if lt := t.Timestamp.Local(); promptDays.newDay(lt) {
					p.DayHeader = lt.Format("January 2, 2006")
					p.DayTitle = lt.Format("Monday, January 2, 2006")
				}
			}
			d.Prompts = append(d.Prompts, p)
		}
```

And replace the turn-card day-detection block:

```go
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

with:

```go
		if !t.Timestamp.IsZero() {
			if lt := t.Timestamp.Local(); turnDays.newDay(lt) {
				tv.DayHeader = lt.Format("Monday, January 2, 2006")
			}
		}
```

4. In `internal/render/assets/report.html.tmpl`, replace the sidebar loop line:

```html
      {{ range .Prompts }}<li><a href="#turn-{{ .Index }}">{{ .Preview }}</a></li>{{ end }}
```

with:

```html
      {{ range .Prompts }}{{ if .DayHeader }}<li class="prompt-day" title="{{ .DayTitle }}">{{ .DayHeader }}</li>{{ end }}<li><a href="#turn-{{ .Index }}"><span class="prompt-preview">{{ .Preview }}</span>{{ if .TimeLabel }}<span class="prompt-time" title="{{ .TimeTitle }}">{{ .TimeLabel }}</span>{{ end }}</a></li>{{ end }}
```

5. In `internal/render/assets/styles.css`, change the `.prompt-list a` rule from:

```css
.prompt-list a { color: var(--text); text-decoration: none; font-size: .85rem; display: block; padding: .25rem .4rem; border-radius: 6px; }
```

to:

```css
.prompt-list a { color: var(--text); text-decoration: none; font-size: .85rem; display: flex; align-items: baseline; gap: .3rem; padding: .25rem .4rem; border-radius: 6px; }
```

and insert directly after the `.prompt-list a:hover` rule:

```css
.prompt-list li.prompt-day { counter-increment: none; font-size: .7rem; text-transform: uppercase; letter-spacing: .08em; color: var(--muted); margin: .75rem 0 .35rem; padding: 0 .4rem; cursor: default; }
.prompt-preview { flex: 1; min-width: 0; }
.prompt-time { color: var(--muted); font-size: .7rem; white-space: nowrap; }
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/render/ -race`
Expected: PASS — the two new tests and all existing tests unmodified (the `dayTracker` refactor is behavior-preserving; `TestBuildViewModelTimeLabelsAndDayHeaders` still passes as written).

- [ ] **Step 5: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/render/render.go internal/render/render_test.go internal/render/assets/report.html.tmpl internal/render/assets/styles.css
git commit -m "feat(render): prompt times and date dividers in the sidebar"
```
