package render

import (
	"testing"
	"time"
)

func TestFormatTokens(t *testing.T) {
	cases := map[int]string{
		0: "0", 192: "192", 12300: "12k", 973000: "973k", 1_200_000: "1.2M",
		// boundary: must never render "1000k"
		999_499: "999k", 999_999: "1.0M", 1_000_000: "1.0M",
	}
	for n, want := range cases {
		if got := formatTokens(n); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	if got := formatCost(12.4); got != "$12.40" {
		t.Errorf("formatCost(12.4) = %q, want $12.40", got)
	}
	if got := formatCost(0.001); got != "<$0.01" {
		t.Errorf("formatCost(0.001) = %q, want <$0.01", got)
	}
}

func TestVersionLink(t *testing.T) {
	const repo = "https://github.com/saigyo/cc-what-have-i-done/"
	const tag = "https://github.com/saigyo/cc-what-have-i-done/releases/tag/"
	cases := []struct{ in, label, href string }{
		{"1.2.3", "v1.2.3", tag + "v1.2.3"},
		{"v1.2.3", "v1.2.3", tag + "v1.2.3"},
		{"0.10.7", "v0.10.7", tag + "v0.10.7"},
		{"dev", "dev build", repo},
		{"", "dev build", repo},
		{"1.2.3-rc1", "dev build", repo},
		{"1.2", "dev build", repo},
		{"v1.2.3.4", "dev build", repo},
		{"vx.y.z", "dev build", repo},
		{"1..3", "dev build", repo},
	}
	for _, c := range cases {
		label, href := versionLink(c.in)
		if label != c.label || href != c.href {
			t.Errorf("versionLink(%q) = (%q, %q), want (%q, %q)", c.in, label, href, c.label, c.href)
		}
	}
}

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
