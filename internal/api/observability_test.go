package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NetSepio/gateway/internal/config"
	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var metricsOnce sync.Once

func registerTestMetrics() {
	metricsOnce.Do(func() {
		metrics.Register()
		metrics.SetGatewayInfo("test", "2.0.1", "deadbeef")
	})
}

func TestHandleHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{cfg: &config.Config{Environment: "test"}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.handleHealthz(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body := w.Body.String(); body != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestHandleVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{cfg: &config.Config{Environment: "test"}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/version", nil)
	s.handleVersion(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{`"product":"erebrus"`, `"service":"gateway"`, `"environment":"test"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q missing %q", body, want)
		}
	}
}

func TestHandleTelemetryEvent(t *testing.T) {
	registerTestMetrics()
	gin.SetMode(gin.TestMode)
	s := &Server{cfg: &config.Config{Environment: "test"}}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/telemetry/event", strings.NewReader(
		`{"platform":"android","event":"vpn_connect_success","status":"success"}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	s.handleTelemetryEvent(c)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	registerTestMetrics()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"netsepio_erebrus_gateway_up",
		"netsepio_erebrus_gateway_build_info",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q:\n%s", want, body)
		}
	}
}
