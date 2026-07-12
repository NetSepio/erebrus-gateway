package api

import (
	"context"
	"net/http"
	"time"

	"github.com/NetSepio/gateway/internal/version"
	"github.com/gin-gonic/gin"
)

// handleHealthz reports liveness (process is running).
func (s *Server) handleHealthz(c *gin.Context) {
	c.String(http.StatusOK, "ok")
}

// handleVersion returns build metadata for deploy verification and dashboards.
func (s *Server) handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"product":     "erebrus",
		"service":     "gateway",
		"environment": s.cfg.Environment,
		"version":     version.Version,
		"tag":         version.Tag,
	})
}

// handleReadyz reports readiness: the DB must be reachable (Redis is optional —
// the gateway degrades to no-cache). Distinct from /healthz (liveness).
func (s *Server) handleReadyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.DB().PingContext(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "db": "down"})
		return
	}
	redis := "disabled"
	if s.cache.Ping(ctx) {
		redis = "up"
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready", "db": "up", "redis": redis})
}
