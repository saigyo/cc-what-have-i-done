package render

import (
	"fmt"
	"strconv"
)

// formatTokens renders a token count compactly: 192, 12k, 973k, 1.2M. The M
// threshold is 999_500 (not 1_000_000) so values that would round up to
// "1000k" render as "1.0M" instead.
func formatTokens(n int) string {
	switch {
	case n >= 999_500:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	default:
		return strconv.Itoa(n)
	}
}

// formatCost renders a USD cost as $X.XX, or <$0.01 for tiny non-zero amounts.
func formatCost(c float64) string {
	if c > 0 && c < 0.005 {
		return "<$0.01"
	}
	return fmt.Sprintf("$%.2f", c)
}
