package api

import (
	"errors"
	"net/http"
	"time"

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

	// trial_consumed means "has the user ever started their one trial" — it is
	// independent of which source currently entitles them (e.g. an NFT holder who
	// previously used the trial still reports trial_consumed=true).
	trialConsumed, err := s.store.HasConsumedTrial(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}

	sub, err := s.store.ActiveSubscription(c, uid)
	if err == nil {
		ok(c, http.StatusOK, gin.H{
			"status": sub.Status, "entitled": true, "plan_id": sub.PlanID,
			"source": sub.Source, "current_period_end": sub.CurrentPeriodEnd,
			"trial_consumed": trialConsumed, "nft_gating": s.nft.Enabled(),
		})
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
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
	sub, err := s.store.StartTrial(c, userID(c), "pro", s.cfg.TrialPeriod)
	if errors.Is(err, store.ErrTrialUsed) {
		fail(c, http.StatusConflict, "trial already used")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to start trial")
		return
	}
	// Qualifying action for referrals: the referee's first trial start awards XP
	// to both parties. Best-effort — never fail the trial on an XP write. The
	// one-trial-per-user index guarantees this path fires at most once per user.
	s.awardReferralXP(c, userID(c))
	ok(c, http.StatusCreated, sub)
}

// awardReferralXP grants referral XP when the user was referred: +referrer to the
// referrer, +referee to the referee (weights from config).
func (s *Server) awardReferralXP(c *gin.Context, refereeID string) {
	referrerID, err := s.store.ReferrerOf(c, refereeID)
	if err != nil || referrerID == "" {
		return
	}
	_ = s.store.AwardXP(c, refereeID, "referral_qualified", s.cfg.XPRefereePoints,
		map[string]any{"role": "referee", "referrer_id": referrerID})
	_ = s.store.AwardXP(c, referrerID, "referral_qualified", s.cfg.XPReferrerPoints,
		map[string]any{"role": "referrer", "referee_id": refereeID})
}

// handleNFTRefresh verifies the caller's wallet holds the gating NFT and
// grants/refreshes a 30-day NFT-sourced entitlement (NFT_GATE_PERIOD) directly,
// regardless of trial state: a new user goes straight to 30 days; a user mid-
// trial is upgraded (the 30d NFT row outlasts the 7d trial, so it becomes the
// active entitlement). Refreshable while held; one per user (idx_subs_one_nft).
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
	// XP driver: holding the NFT earns XP once per month (best-effort).
	month := time.Now().UTC().Format("200601")
	_, _ = s.store.AwardXPOnce(c, u.ID, "nft_held", s.cfg.XPNFTHeld,
		map[string]any{"month": month}, "nft_held:"+u.ID+":"+month)
	ok(c, http.StatusOK, sub)
}
