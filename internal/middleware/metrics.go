package middleware

import (
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/gin-gonic/gin"
)

// Metrics records HTTP request count, errors, and latency using route patterns.
func Metrics(environment string) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}

		metrics.ObserveHTTPRequest(
			c.Request.Method,
			route,
			c.Writer.Status(),
			DetectClient(c),
			environment,
			started,
		)
	}
}

// DetectClient reads the bounded client label from X-Erebrus-Client.
func DetectClient(c *gin.Context) string {
	client := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Erebrus-Client")))
	switch client {
	case "webapp", "android", "ios", "node":
		return client
	default:
		return "unknown"
	}
}