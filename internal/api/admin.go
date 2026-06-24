package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// handleAdminStats returns the platform overview.
func (s *Server) handleAdminStats(c *gin.Context) {
	nodesByStatus, _ := s.store.CountNodesByStatus(c)
	users, _ := s.store.CountUsers(c)
	orgs, _ := s.store.CountOrgs(c)
	subsByPlan, _ := s.store.CountActiveSubscriptionsByPlan(c)
	since := time.Now().AddDate(0, 0, -30)
	rx, tx, _ := s.store.TotalBandwidth(c, since)

	ok(c, http.StatusOK, gin.H{
		"nodes": gin.H{
			"by_status": nodesByStatus,
			"connected": s.hub.Online(),
		},
		"users":         gin.H{"total": users},
		"orgs":          gin.H{"total": orgs},
		"subscriptions": gin.H{"by_plan": subsByPlan},
		"traffic_30d":   gin.H{"rx_bytes": rx, "tx_bytes": tx},
	})
}

// handleAdminUsers lists users (paginated).
func (s *Server) handleAdminUsers(c *gin.Context) {
	limit, offset := pagination(c)
	users, err := s.store.ListUsers(c, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list users")
		return
	}
	ok(c, http.StatusOK, gin.H{"users": users, "limit": limit, "offset": offset})
}

// handleAdminSubscriptions returns active subscription counts by plan.
func (s *Server) handleAdminSubscriptions(c *gin.Context) {
	byPlan, err := s.store.CountActiveSubscriptionsByPlan(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscriptions")
		return
	}
	ok(c, http.StatusOK, gin.H{"active_by_plan": byPlan})
}

func pagination(c *gin.Context) (limit, offset int) {
	limit = 50
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	if v, err := strconv.Atoi(c.Query("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}
