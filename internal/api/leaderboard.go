package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// handleLeaderboard returns the top users by metric (xp|referrals) over a period
// (all|30d), Redis-cached, plus the caller's own rank.
func (s *Server) handleLeaderboard(c *gin.Context) {
	metric := c.DefaultQuery("metric", "xp")
	if metric != "xp" && metric != "referrals" {
		fail(c, http.StatusBadRequest, "metric must be xp or referrals")
		return
	}
	period := c.DefaultQuery("period", "all")
	var since *time.Time
	switch period {
	case "all":
	case "30d":
		t := time.Now().Add(-30 * 24 * time.Hour)
		since = &t
	default:
		fail(c, http.StatusBadRequest, "period must be all or 30d")
		return
	}
	limit := clampInt(c.Query("limit"), 50, 1, 100)
	offset := clampInt(c.Query("offset"), 0, 0, 1_000_000)

	key := "leaderboard:" + metric + ":" + period + ":" + strconv.Itoa(offset) + ":" + strconv.Itoa(limit)
	var entries []store.LeaderEntry
	if hit, _ := s.cache.GetJSON(c, key, &entries); !hit {
		var err error
		entries, err = s.store.Leaderboard(c, metric, since, limit, offset)
		if err != nil {
			fail(c, http.StatusInternalServerError, "failed to load leaderboard")
			return
		}
		_ = s.cache.SetJSON(c, key, entries, 30*time.Second)
	}

	rows := make([]gin.H, 0, len(entries))
	for i, e := range entries {
		rows = append(rows, gin.H{"rank": offset + i + 1, "wallet": truncWallet(e.Wallet), "value": e.Value})
	}
	resp := gin.H{"metric": metric, "period": period, "entries": rows}
	if rank, val, err := s.store.MyRank(c, metric, since, userID(c)); err == nil {
		resp["my_rank"] = rank
		resp["my_value"] = val
	}
	ok(c, http.StatusOK, resp)
}

type claimReq struct {
	Reward string `json:"reward"`
}

// handleRankClaim retains the legacy route without creating personal product
// entitlement. Organization plans and seats are the only access source.
func (s *Server) handleRankClaim(c *gin.Context) {
	var req claimReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Reward == "" {
		fail(c, http.StatusBadRequest, "reward is required")
		return
	}
	fail(c, http.StatusGone, "personal entitlement rewards are retired; access is managed by organization plans and seats")
}

// clampInt parses s as an int, applying a default for empty/invalid and clamping
// to [lo, hi].
func clampInt(s string, def, lo, hi int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// clientIP returns the caller's IP (gin honors X-Forwarded-For from trusted
// proxies; S8 hardening pins the trusted set).
func clientIP(c *gin.Context) string { return c.ClientIP() }
