package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// reqMetrics holds lightweight in-process request counters (no Prometheus client
// dependency — rendered in the text exposition format at /metrics).
type reqMetrics struct {
	total atomic.Int64
	c2xx  atomic.Int64
	c4xx  atomic.Int64
	c5xx  atomic.Int64
}

// metricsMiddleware counts responses by status class.
func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		s.metrics.total.Add(1)
		switch c.Writer.Status() / 100 {
		case 2:
			s.metrics.c2xx.Add(1)
		case 4:
			s.metrics.c4xx.Add(1)
		case 5:
			s.metrics.c5xx.Add(1)
		}
	}
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

// handleMetrics renders Prometheus text exposition for the key signals: build
// info, request counts by class, connected nodes, and DB pool stats.
func (s *Server) handleMetrics(c *gin.Context) {
	var b strings.Builder
	metricLine(&b, "gateway_build_info", `{version="`+s.cfg.Version+`"}`, 1)
	metricLine(&b, "gateway_http_requests_total", `{class="2xx"}`, s.metrics.c2xx.Load())
	metricLine(&b, "gateway_http_requests_total", `{class="4xx"}`, s.metrics.c4xx.Load())
	metricLine(&b, "gateway_http_requests_total", `{class="5xx"}`, s.metrics.c5xx.Load())
	metricLine(&b, "gateway_http_requests_total", `{class="all"}`, s.metrics.total.Load())
	metricLine(&b, "gateway_ws_nodes_connected", "", int64(s.hub.Online()))

	st := s.store.DB().Stats()
	metricLine(&b, "gateway_db_open_connections", "", int64(st.OpenConnections))
	metricLine(&b, "gateway_db_in_use", "", int64(st.InUse))
	metricLine(&b, "gateway_db_idle", "", int64(st.Idle))
	metricLine(&b, "gateway_db_wait_count", "", st.WaitCount)

	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
}

func metricLine(b *strings.Builder, name, labels string, val int64) {
	b.WriteString(name)
	b.WriteString(labels)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(val, 10))
	b.WriteByte('\n')
}
