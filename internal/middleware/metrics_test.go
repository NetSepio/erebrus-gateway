package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDetectClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct{ header, want string }{
		{"webapp", "webapp"},
		{"android", "android"},
		{"ios", "ios"},
		{"node", "node"},
		{"desktop", "unknown"},
		{"", "unknown"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			c.Request.Header.Set("X-Erebrus-Client", tc.header)
		}
		if got := DetectClient(c); got != tc.want {
			t.Errorf("DetectClient(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}