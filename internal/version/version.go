package version

import "fmt"

// These variables are set at build time via -ldflags.
// Defaults are used for local dev builds (go run / go build without ldflags).
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// String returns the full version string shown to users.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, BuildDate)
}
