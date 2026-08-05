// Package version holds build-time version information, injected via ldflags.
package version

var (
	Version   = "unknown version"
	BuildTime = "unknown time"
	CommitSHA = ""
)
