package api

import (
	"net/http"

	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/gin-gonic/gin"
)

type telemetryEvent struct {
	Platform string `json:"platform"`
	Event    string `json:"event"`
	Status   string `json:"status"`
}

// handleTelemetryEvent ingests safe, whitelisted app telemetry events.
func (s *Server) handleTelemetryEvent(c *gin.Context) {
	var event telemetryEvent
	if err := c.ShouldBindJSON(&event); err != nil {
		fail(c, http.StatusBadRequest, "invalid payload")
		return
	}

	platform := metrics.NormalizePlatform(event.Platform)
	ev := metrics.NormalizeEvent(event.Event)
	status := metrics.NormalizeStatus(event.Status)

	metrics.AppEventsTotal.
		WithLabelValues(platform, ev, status, s.cfg.Environment).
		Inc()

	c.AbortWithStatus(http.StatusNoContent)
}
