package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// handleAdminStats returns the platform overview.
func (s *Server) handleAdminStats(c *gin.Context) {
	nodesByStatus, _ := s.store.CountNodesByStatus(c)
	users, _ := s.store.CountUsers(c)
	orgs, _ := s.store.CountOrgs(c)
	subsByPlan, _ := s.store.CountLegacyActiveSubscriptionsByPlan(c)
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
	byPlan, err := s.store.CountLegacyActiveSubscriptionsByPlan(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscriptions")
		return
	}
	ok(c, http.StatusOK, gin.H{"active_by_plan": byPlan})
}

// handleAdminUser returns a single user profile, current plan, and pending deletion request.
func (s *Server) handleAdminUser(c *gin.Context) {
	id := c.Param("id")
	u, err := s.store.GetUser(c, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "user not found")
			return
		}
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	plan, _ := s.store.GetUserPlan(c, id)
	req, _ := s.store.GetDeletionRequestByUserID(c, id)
	ok(c, http.StatusOK, gin.H{
		"id":              u.ID,
		"wallet_address":  u.WalletAddress,
		"chain":           u.Chain,
		"role":            u.Role,
		"email":           u.Email,
		"email_verified":  u.EmailVerified,
		"name":            u.Name,
		"profile_picture": u.ProfilePicture,
		"created_at":      u.CreatedAt,
		"plan":            plan,
		"deletion_request": req,
	})
}

// handleAdminUserOrgs lists the organizations the user is a member of.
func (s *Server) handleAdminUserOrgs(c *gin.Context) {
	id := c.Param("id")
	orgs, err := s.store.ListOrgsForUser(c, id)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user orgs")
		return
	}
	ok(c, http.StatusOK, gin.H{"orgs": orgs})
}

type adminUserPlanReq struct {
	PlanID string `json:"plan_id" binding:"required"`
}

// handleAdminUserPlan sets a user's plan subscription.
func (s *Server) handleAdminUserPlan(c *gin.Context) {
	id := c.Param("id")
	var req adminUserPlanReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "plan_id required")
		return
	}
	if _, err := s.store.GetPlan(c, req.PlanID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusBadRequest, "unknown plan")
			return
		}
		fail(c, http.StatusInternalServerError, "failed to validate plan")
		return
	}
	if _, err := s.store.SetUserPlan(c, id, req.PlanID); err != nil {
		fail(c, http.StatusInternalServerError, "failed to set plan")
		return
	}
	ok(c, http.StatusOK, gin.H{"status": "updated"})
}

// handleAdminDeletionRequests lists pending and fulfilled account-deletion requests.
func (s *Server) handleAdminDeletionRequests(c *gin.Context) {
	limit, offset := pagination(c)
	requests, total, err := s.store.ListDeletionRequests(c, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list deletion requests")
		return
	}
	ok(c, http.StatusOK, gin.H{"requests": requests, "total": total, "limit": limit, "offset": offset})
}

// handleAdminFulfillDeletionRequest fulfills a deletion request and deletes the user account.
func (s *Server) handleAdminFulfillDeletionRequest(c *gin.Context) {
	id := c.Param("id")
	email, err := s.store.FulfillDeletionRequest(c, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "request not found or already fulfilled")
			return
		}
		fail(c, http.StatusInternalServerError, "failed to fulfill deletion request")
		return
	}
	if email != "" && s.mailer != nil && s.mailer.Enabled() {
		_ = s.mailer.SendDeletionProcessed(c, email)
	}
	ok(c, http.StatusOK, gin.H{"status": "fulfilled"})
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
