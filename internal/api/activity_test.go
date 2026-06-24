package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClientApp(t *testing.T) {
	cases := []struct {
		hint, ua, want string
	}{
		{"erebrus-ios/1.0", "", "ios"},
		{"erebrus-android/1.0", "", "android"},
		{"erebrus-desktop", "", "desktop"},
		{"web", "", "web"},
		{"", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)", "ios"},
		{"", "Mozilla/5.0 (Linux; Android 14)", "android"},
		{"", "Mozilla/5.0 (Macintosh)", "web"},
		{"", "curl/8.0", ""},
	}
	gin.SetMode(gin.TestMode)
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/", nil)
		if tc.hint != "" {
			c.Request.Header.Set("X-Erebrus-Client", tc.hint)
		}
		if tc.ua != "" {
			c.Request.Header.Set("User-Agent", tc.ua)
		}
		if got := clientApp(c); got != tc.want {
			t.Errorf("clientApp(hint=%q ua=%q) = %q, want %q", tc.hint, tc.ua, got, tc.want)
		}
	}
}

// TestActivityActionsValid guards the audit action map: non-empty keys/values
// and the METHOD-prefix shape that activityLog looks up.
func TestActivityActionsValid(t *testing.T) {
	if len(activityActions) == 0 {
		t.Fatal("activityActions is empty")
	}
	for k, v := range activityActions {
		if v == "" {
			t.Errorf("empty action for %q", k)
		}
		// key is "METHOD /path"
		sp := -1
		for i := 0; i < len(k); i++ {
			if k[i] == ' ' {
				sp = i
				break
			}
		}
		if sp <= 0 || sp+1 >= len(k) || k[sp+1] != '/' {
			t.Errorf("malformed action key %q (want \"METHOD /path\")", k)
		}
	}
}
