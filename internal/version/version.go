// Package version holds the gateway build version (injected via -ldflags).
package version

// Version is set at link time: -X github.com/NetSepio/gateway/internal/version.Version=<tag>
var Version = "dev"