package render

import "testing"

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
