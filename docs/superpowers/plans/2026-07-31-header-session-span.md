# Header Session Span & Render Time Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The report header's meta line shows the session's end via a same-day duration (`2026-07-03 17:03 (3h 12m)`) or cross-day range (`… → …`), plus a `rendered <timestamp>` span.

**Architecture:** A pure `sessionSpan(start, end)` helper in `format.go` returns the visible span text and its hover title for the four spec cases; `formatDuration` renders whole-minute durations. `buildViewModel` replaces the `StartedAt` view field with `SessionSpan`/`SessionSpanTitle`/`GeneratedAt`, the latter from a `timeNow` package hook stubbed in tests. The template swaps one span for two.

**Tech Stack:** Go stdlib only (`time`, `fmt`); plain `testing`.

## Global Constraints

- Display cases, exactly and in precedence order: (1) zero `StartedAt` → empty span, omitted; (2) zero `EndedAt` or duration under one minute → formatted start alone, title `session start`; (3) same local calendar day → `<start> (<duration>)`, title `last message <end>`; (4) different local days → `<start> → <end>`, title `session start → last message`.
- Timestamp format `2006-01-02 15:04`, `.Local()` conversion at render time. Duration truncated to whole minutes: `3h 12m`, or `45m` under an hour.
- Same-day comparison uses the two local `Date()` triples (consistent with `dayTracker`).
- The render timestamp comes from a package-level `var timeNow = time.Now`, overridden in tests.
- No parser/model changes; `Session.EndedAt` and `Duration()` already exist. No CSS/JS changes.
- Existing tests pass unmodified, with one exception: `TestBuildViewModelStartedAtIsLocal` (render_test.go:694) asserts the removed `StartedAt` view field and is replaced by the new session-span test. Any other failing test is a defect to investigate, not to edit.
- No new go.mod dependencies. gofmt-clean, `go vet ./...` clean, tests pass with `-race`.
- Commit messages end with the standard `Co-Authored-By` + `Claude-Session` trailers (implementer uses its own model name).

---

### Task 1: sessionSpan helper, view-model fields, template

**Files:**
- Modify: `internal/render/format.go` (add `metaTimeLayout`, `formatDuration`, `sessionSpan`; add `fmt` to imports if absent)
- Modify: `internal/render/render.go` (`viewData` — replace `StartedAt` field; `buildViewModel` — replace the `StartedAt` block; add `timeNow` var)
- Modify: `internal/render/assets/report.html.tmpl` (the `.meta` start-time span)
- Test: `internal/render/format_test.go`, `internal/render/render_test.go`

**Interfaces:**
- Consumes: `model.Session.StartedAt`/`.EndedAt` (`time.Time`, UTC, zero when absent); existing `timeParts` stays untouched.
- Produces: `metaTimeLayout = "2006-01-02 15:04"` (const, package render); `formatDuration(d time.Duration) string`; `sessionSpan(start, end time.Time) (span, title string)`; `var timeNow = time.Now`; `viewData.SessionSpan`, `.SessionSpanTitle`, `.GeneratedAt` (all `string`; `StartedAt` field deleted).

- [ ] **Step 1: Write the failing tests**

Append to `internal/render/format_test.go`:

```go
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{45 * time.Minute, "45m"},
		{3*time.Hour + 12*time.Minute, "3h 12m"},
		{2 * time.Hour, "2h 0m"},
		{90 * time.Second, "1m"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestSessionSpan(t *testing.T) {
	start := time.Date(2026, 7, 3, 17, 3, 0, 0, time.Local)
	sameDayEnd := start.Add(3*time.Hour + 12*time.Minute)
	nextDayEnd := start.AddDate(0, 0, 28).Add(2*time.Hour + 9*time.Minute)

	cases := []struct {
		name       string
		start, end time.Time
		wantSpan   string
		wantTitle  string
	}{
		{"zero start", time.Time{}, sameDayEnd, "", ""},
		{"zero end", start, time.Time{}, "2026-07-03 17:03", "session start"},
		{"sub-minute", start, start.Add(30 * time.Second), "2026-07-03 17:03", "session start"},
		{"same day", start, sameDayEnd, "2026-07-03 17:03 (3h 12m)", "last message 2026-07-03 20:15"},
		{"cross day", start, nextDayEnd, "2026-07-03 17:03 → 2026-07-31 19:12", "session start → last message"},
		{"UTC inputs convert to local", start.UTC(), sameDayEnd.UTC(), "2026-07-03 17:03 (3h 12m)", "last message 2026-07-03 20:15"},
	}
	for _, c := range cases {
		span, title := sessionSpan(c.start, c.end)
		if span != c.wantSpan || title != c.wantTitle {
			t.Errorf("%s: sessionSpan = %q, %q; want %q, %q", c.name, span, title, c.wantSpan, c.wantTitle)
		}
	}
}
```

In `internal/render/render_test.go`, replace `TestBuildViewModelStartedAtIsLocal` (at :694, the whole function) with:

```go
func TestBuildViewModelSessionSpanAndGeneratedAt(t *testing.T) {
	orig := timeNow
	gen := time.Date(2026, 7, 31, 20, 52, 0, 0, time.Local)
	timeNow = func() time.Time { return gen.UTC() } // returns UTC; view must localize
	t.Cleanup(func() { timeNow = orig })

	start := time.Date(2026, 7, 3, 17, 3, 0, 0, time.Local)
	end := start.Add(3*time.Hour + 12*time.Minute)
	sess := model.Session{StartedAt: start.UTC(), EndedAt: end.UTC()} // stored as UTC, like real transcripts
	d := buildViewModel(sess, "t", Options{}, pageInfo{}, newAgentLinks(nil, ""))
	if want := start.Format("2006-01-02 15:04") + " (3h 12m)"; d.SessionSpan != want {
		t.Errorf("SessionSpan = %q, want local-time %q", d.SessionSpan, want)
	}
	if want := "last message " + end.Format("2006-01-02 15:04"); d.SessionSpanTitle != want {
		t.Errorf("SessionSpanTitle = %q, want %q", d.SessionSpanTitle, want)
	}
	if want := gen.Format("2006-01-02 15:04"); d.GeneratedAt != want {
		t.Errorf("GeneratedAt = %q, want local-time %q", d.GeneratedAt, want)
	}
}
```

Append to `internal/render/render_test.go`:

```go
func TestSiteRendersSessionSpanAndRenderTime(t *testing.T) {
	orig := timeNow
	gen := time.Date(2026, 7, 31, 20, 52, 0, 0, time.Local)
	timeNow = func() time.Time { return gen }
	t.Cleanup(func() { timeNow = orig })

	start := time.Date(2026, 7, 29, 14, 0, 0, 0, time.Local)
	end := start.Add(90 * time.Minute)
	sess := model.Session{
		StartedAt: start,
		EndedAt:   end,
		Turns: []model.Turn{{Kind: model.TurnUser, Timestamp: start,
			Blocks: []model.Block{{Type: model.BlockText, Text: "hi"}}}},
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
	wantSpan := `<span title="last message ` + end.Format("2006-01-02 15:04") + `">` +
		start.Format("2006-01-02 15:04") + ` (1h 30m)</span>`
	if !strings.Contains(out, wantSpan) {
		t.Errorf("session span missing from report; want %s", wantSpan)
	}
	if !strings.Contains(out, "rendered "+gen.Format("2006-01-02 15:04")) {
		t.Error("rendered timestamp missing from report")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/render/ -run 'TestFormatDuration|TestSessionSpan|TestBuildViewModelSessionSpanAndGeneratedAt|TestSiteRendersSessionSpanAndRenderTime'`
Expected: FAIL to build — `undefined: formatDuration`, `undefined: sessionSpan`, `undefined: timeNow`, `d.SessionSpan undefined`.

- [ ] **Step 3: Implement**

1. In `internal/render/format.go`: ensure `"fmt"` and `"time"` are imported, and append:

```go
// metaTimeLayout is the timestamp format used in the header meta line.
const metaTimeLayout = "2006-01-02 15:04"

// formatDuration renders d truncated to whole minutes: "3h 12m", "45m".
func formatDuration(d time.Duration) string {
	mins := int(d.Minutes())
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh %dm", mins/60, mins%60)
}

// sessionSpan formats the header's session-time span and its hover title
// from a session's first and last message times, in local time. Cases:
// zero start → both empty; zero end or sub-minute duration → bare start;
// same local day → "start (3h 12m)" titled with the last-message time;
// different days → "start → end" titled with an explanation of the arrow.
func sessionSpan(start, end time.Time) (span, title string) {
	if start.IsZero() {
		return "", ""
	}
	ls := start.Local()
	s := ls.Format(metaTimeLayout)
	if end.IsZero() || end.Sub(start) < time.Minute {
		return s, "session start"
	}
	le := end.Local()
	sy, sm, sd := ls.Date()
	ey, em, ed := le.Date()
	if sy == ey && sm == em && sd == ed {
		return s + " (" + formatDuration(end.Sub(start)) + ")", "last message " + le.Format(metaTimeLayout)
	}
	return s + " → " + le.Format(metaTimeLayout), "session start → last message"
}
```

2. In `internal/render/render.go`:

   a. Add near the top of the file, directly above `type Options`:

```go
// timeNow is stubbed in tests to pin the render timestamp.
var timeNow = time.Now
```

   b. In `viewData`, replace the field `StartedAt    string` with:

```go
	SessionSpan      string // "2026-07-03 17:03 (3h 12m)" or "… → …"; "" without timestamps
	SessionSpanTitle string // hover title matching the span's form
	GeneratedAt      string // render timestamp, always set
```

   c. In `buildViewModel`, replace:

```go
	if !s.StartedAt.IsZero() {
		d.StartedAt = s.StartedAt.Local().Format("2006-01-02 15:04")
	}
```

with:

```go
	d.SessionSpan, d.SessionSpanTitle = sessionSpan(s.StartedAt, s.EndedAt)
	d.GeneratedAt = timeNow().Local().Format(metaTimeLayout)
```

3. In `internal/render/assets/report.html.tmpl`, replace:

```html
        <span>{{ .StartedAt }}</span>
```

with:

```html
        {{ if .SessionSpan }}<span title="{{ .SessionSpanTitle }}">{{ .SessionSpan }}</span>{{ end }}
        <span>rendered {{ .GeneratedAt }}</span>
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/render/ -race`
Expected: PASS — the four new/replaced tests and every other existing test unmodified.

- [ ] **Step 5: Full verification**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: no gofmt output, vet clean, all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/render/format.go internal/render/format_test.go internal/render/render.go internal/render/render_test.go internal/render/assets/report.html.tmpl
git commit -m "feat(render): session span with duration and render time in the header"
```
