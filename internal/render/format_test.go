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
