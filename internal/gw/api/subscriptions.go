package api

import (
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/NetSepio/gateway/internal/gw/token"
	"github.com/gin-gonic/gin"
)

// handlePlans lists subscription plans (public). In v2.0 plans describe limits;
// there is no paid checkout — entitlement comes from the trial or NFT gating.
func (s *Server) handlePlans(c *gin.Context) {
	plans, err := s.store.ListPlans(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list plans")
		return
	}
	ok(c, http.StatusOK, plans)
}

// handleMySubscription returns the caller's current entitlement.
func (s *Server) handleMySubscription(c *gin.Context) {
	uid := userID(c)
	if c.GetString(ctxRole) == token.RoleAdmin {
		ok(c, http.StatusOK, gin.H{
			"status": "active", "entitled": true, "plan_id": "pro",
			"source": "admin", "trial_consumed": false, "nft_gating": s.nft.Enabled(),
		})
		return
	}

	sub, err := s.store.ActiveSubscription(c, uid)
	if err == nil {
		ok(c, http.StatusOK, gin.H{
			"status": sub.Status, "entitled": true, "plan_id": sub.PlanID,
			"source": sub.Source, "current_period_end": sub.CurrentPeriodEnd,
			"trial_consumed": sub.Source == "trial", "nft_gating": s.nft.Enabled(),
		})
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}

	trialConsumed, err := s.store.HasConsumedTrial(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}
	resp := gin.H{
		"entitled": false, "trial_consumed": trialConsumed,
		"nft_gating": s.nft.Enabled(),
	}
	if last, err := s.store.LastSubscription(c, uid); err == nil {
		resp["status"] = "expired"
		resp["plan_id"] = last.PlanID
		resp["source"] = last.Source
		resp["current_period_end"] = last.CurrentPeriodEnd
	} else if errors.Is(err, store.ErrNotFound) {
		resp["status"] = "none"
	}
	ok(c, http.StatusOK, resp)
}

// handleStartTrial grants the one-time free trial (on the 'pro' plan).
func (s *Server) handleStartTrial(c *gin.Context) {
	sub, err := s.store.StartTrial(c, userID(c), "pro", trialPeriod)
	if errors.Is(err, store.ErrTrialUsed) {
		fail(c, http.StatusConflict, "trial already used")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to start trial")
		return
	}
	ok(c, http.StatusCreated, sub)
}

// handleNFTRefresh verifies the caller's wallet holds the gating NFT and
// grants/refreshes an NFT-sourced entitlement.
func (s *Server) handleNFTRefresh(c *gin.Context) {
	if !s.nft.Enabled() {
		fail(c, http.StatusServiceUnavailable, "NFT gating is not configured")
		return
	}
	u, err := s.store.GetUser(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	if u.WalletAddress == "" {
		fail(c, http.StatusBadRequest, "no wallet on account")
		return
	}
	owns, err := s.nft.Owns(c, u.WalletAddress)
	if err != nil {
		fail(c, http.StatusBadGateway, "NFT ownership check failed")
		return
	}
	if !owns {
		fail(c, http.StatusForbidden, "wallet does not hold the required NFT")
		return
	}
	sub, err := s.store.GrantNFTSubscription(c, u.ID, s.cfg.NFTGatePlanID, s.cfg.NFTGatePeriod)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to grant entitlement")
		return
	}
	ok(c, http.StatusOK, sub)
}
