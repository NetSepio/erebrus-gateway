// Package version holds gateway build metadata (injected via -ldflags).
package version

// Version is the auto-incremented release version (e.g. 2.0.428).
var Version = "dev"

// Tag is the source commit hash (short SHA in CI builds).
var Tag = "unknown"