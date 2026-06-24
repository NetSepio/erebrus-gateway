package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/gin-gonic/gin"
)

// activityActions maps "METHOD <FullPath>" to a stable action name for the audit
// log. Only mutating, authenticated routes are listed (read-only GETs and the
// public auth challenge are not logged here; login + email verify are logged
// explicitly in their handlers).
var activityActions = map[string]string{
	"PATCH /api/v2/account/profile":          "profile.update",
	"POST /api/v2/vpn/clients":               "vpn.client.provision",
	"DELETE /api/v2/vpn/clients/:id":         "vpn.client.delete",
	"POST /api/v2/subscriptions/trial":       "subscription.trial_start",
	"POST /api/v2/subscriptions/nft/refresh": "subscription.nft_refresh",
	"POST /api/v2/orgs":                      "org.create",
	"POST /api/v2/orgs/:id/members":          "org.member.add",
	"POST /api/v2/orgs/:id/apikeys":          "apikey.create",
	"DELETE /api/v2/orgs/:id/apikeys/:keyId": "apikey.revoke",
	"POST /api/v2/rank/claim":                "rank.claim",
	"POST /api/v2/social/telegram":           "social.verify.telegram",
	"POST /api/v2/social/x":                  "social.verify.x",
	"POST /api/v2/admin/nodes/:id/command":   "admin.node.command",
	"POST /api/v2/admin/nodes/:id/min_tier":  "admin.node.min_tier",
	"POST /api/v2/admin/perks":               "admin.perk.upsert",
	"POST /api/v2/admin/perks/:id/grant":     "admin.perk.grant",
}

// activityLog records an audit entry after a successful (2xx) mutating request
// on an authenticated route. Place it after authUser so the user id is set.
func (s *Server) activityLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method == http.MethodGet || c.Writer.Status()/100 != 2 {
			return
		}
		action := activityActions[c.Request.Method+" "+c.FullPath()]
		if action == "" {
			return
		}
		uid := userID(c)
		if uid == "" {
			return
		}
		target := c.Param("keyId")
		if target == "" {
			target = c.Param("id")
		}
		s.logActivity(c, uid, action, target)
	}
}

// logActivity writes one audit record (best-effort; never fails the request).
func (s *Server) logActivity(c *gin.Context, userID, action, target string) {
	_ = s.store.LogActivity(c, store.ActivityEntry{
		UserID: userID, Action: action, Target: target,
		IP: clientIP(c), UserAgent: c.GetHeader("User-Agent"),
		Device: c.GetHeader("X-Erebrus-Client"), App: clientApp(c),
	})
}

// clientApp classifies the caller platform from the device hint, then the UA.
func clientApp(c *gin.Context) string {
	hint := strings.ToLower(c.GetHeader("X-Erebrus-Client"))
	switch {
	case strings.Contains(hint, "ios"):
		return "ios"
	case strings.Contains(hint, "android"):
		return "android"
	case strings.Contains(hint, "desktop"):
		return "desktop"
	case strings.Contains(hint, "web"):
		return "web"
	}
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	switch {
	case strings.Contains(ua, "android"):
		return "android"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ios"):
		return "ios"
	case strings.Contains(ua, "mozilla"):
		return "web"
	}
	return ""
}

// handleAccountActivity returns the caller's own activity, newest first.
func (s *Server) handleAccountActivity(c *gin.Context) {
	limit := clampInt(c.Query("limit"), 50, 1, 200)
	entries, err := s.store.ListUserActivity(c, userID(c), c.Query("cursor"), limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load activity")
		return
	}
	ok(c, http.StatusOK, activityPage(entries, limit, false))
}

// handleAdminActivity returns fleet-wide activity (admin), newest first.
func (s *Server) handleAdminActivity(c *gin.Context) {
	limit := clampInt(c.Query("limit"), 50, 1, 200)
	entries, err := s.store.ListAllActivity(c, c.Query("cursor"), limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load activity")
		return
	}
	ok(c, http.StatusOK, activityPage(entries, limit, true))
}

// activityPage builds the paginated response, truncating actor wallets for admin.
func activityPage(entries []store.ActivityEntry, limit int, admin bool) gin.H {
	if admin {
		for i := range entries {
			entries[i].Wallet = truncWallet(entries[i].Wallet)
		}
	}
	resp := gin.H{"activity": entries}
	if len(entries) == limit && limit > 0 {
		resp["next_cursor"] = entries[len(entries)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	return resp
}
