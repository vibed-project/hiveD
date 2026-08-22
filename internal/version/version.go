// Package version holds build-time metadata injected via -ldflags.
package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
