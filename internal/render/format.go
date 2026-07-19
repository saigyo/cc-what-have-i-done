package render

import (
	"fmt"
	"strconv"
	"strings"
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

// repoURL is the project home; versionLink points here when no release tag
// can be derived from the build version.
const repoURL = "https://github.com/saigyo/cc-what-have-i-done"

// versionLink maps a build version to the label and href shown under the
// brand in the topbar. "1.2.3" or "v1.2.3" (GoReleaser strips the tag's v
// prefix) yield ("v1.2.3", …/releases/tag/v1.2.3); anything else — dev
// builds, pre-releases, malformed strings — yields ("dev build", the repo).
func versionLink(version string) (label, href string) {
	v := strings.TrimPrefix(version, "v")
	if !isReleaseVersion(v) {
		return "dev build", repoURL + "/"
	}
	return "v" + v, repoURL + "/releases/tag/v" + v
}

// isReleaseVersion reports whether v is exactly <digits>.<digits>.<digits>.
func isReleaseVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return false
			}
		}
	}
	return true
}
