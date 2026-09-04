// Package version holds build-time identification data.
package version

import (
	"fmt"
	"runtime"
)

// These values are overridden at build time via -ldflags -X.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human readable one-line version banner.
func String() string {
	return fmt.Sprintf("aiusagemonitor %s (commit %s, built %s, %s/%s, %s)",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH, runtime.Version())
}

// Short returns just the semantic version.
func Short() string { return Version }
