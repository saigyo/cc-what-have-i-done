// Package ccwhid exposes repo-root assets that ship inside the binary.
package ccwhid

import _ "embed"

// ThirdPartyLicenses is the complete third-party notices file, generated
// by tools/gen-licenses and verified fresh in CI.
//
//go:embed THIRD_PARTY_LICENSES.txt
var ThirdPartyLicenses string
