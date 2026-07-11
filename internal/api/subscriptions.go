package api

import (
	"net/http"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/gin-gonic/gin"
)

// handlePlans lists organization plan limits. Personal subscription, trial, and
// NFT rows are retained only for compatibility and do not grant access.
func (s *Server) handlePlans(c *gin.Context) {
	plans, err := s.store.ListPlans(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list plans")
		return
	}
	ok(c, http.StatusOK, plans)
}

// orgEntitlementResponse builds the organization-derived compatibility payload
// returned by the legacy subscription endpoints while the webapp migrates. It is
// the single source of truth: organization membership only — personal trials,
// per-user subscriptions, and NFT grants are no longer consulted for access. The
// legacy trial_consumed flag is reported read-only (informational) from retained
// data; it never affects entitlement.
func (s *Server) orgEntitlementResponse(c *gin.Context, uid string) (gin.H, error) {
	ent, err := s.store.ResolveDropEntitlement(c, uid)
	if err != nil {
		return nil, err
	}
	orgPlan, err := s.store.UserOrgVPNPlan(c, uid)
	if err != nil {
		return nil, err
	}
	orgMember, _ := s.store.UserHasActiveOrgMembership(c, uid)
	trialConsumed, _ := s.store.HasConsumedTrial(c, uid)

	entitled := orgPlan != ""
	planID, status := orgPlan, "active"
	if !entitled {
		planID, status = store.DropTierFree, "inactive"
	}
	return gin.H{
		"status": status, "entitled": entitled, "plan_id": planID,
		"source": "org", "org_member": orgMember,
		"drop_tier":                 ent.Tier,
		"drop_public_storage_bytes": ent.PublicStorageBytes,
		"entitlement_org_id":        ent.EntitlementOrgID,
		"trial_consumed":            trialConsumed,
		"nft_gating":                s.nft.Enabled(),
	}, nil
}

// handleMySubscription returns the caller's current entitlement, derived solely
// from organization membership.
func (s *Server) handleMySubscription(c *gin.Context) {
	uid := userID(c)
	if c.GetString(ctxRole) == token.RoleAdmin {
		ok(c, http.StatusOK, gin.H{
			"status": "active", "entitled": true, "plan_id": "pro",
			"source": "admin", "trial_consumed": false, "nft_gating": s.nft.Enabled(),
		})
		return
	}
	resp, err := s.orgEntitlementResponse(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}
	ok(c, http.StatusOK, resp)
}

// handleStartTrial is retired. Trials are no longer granted; entitlement comes
// from organization membership. The route is kept as a no-op that returns the
// organization-derived entitlement so the webapp keeps working during migration.
func (s *Server) handleStartTrial(c *gin.Context) {
	resp, err := s.orgEntitlementResponse(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}
	resp["trial_retired"] = true
	ok(c, http.StatusOK, resp)
}

// awardReferralXP grants referral XP when the user was referred: +referrer to the
// referrer, +referee to the referee (weights from config). Dedup-keyed per
// referee, so every path that can complete a referral (trial start, signup
// binding, late code redemption) may call it without double-awarding.
func (s *Server) awardReferralXP(c *gin.Context, refereeID string) {
	referrerID, err := s.store.ReferrerOf(c, refereeID)
	if err != nil || referrerID == "" {
		return
	}
	plat := s.platform.Snapshot()
	_, _ = s.store.AwardXPOnce(c, refereeID, "referral_qualified", plat.XPRefereePoints,
		map[string]any{"role": "referee", "referrer_id": referrerID},
		"referral:"+refereeID+":referee")
	_, _ = s.store.AwardXPOnce(c, referrerID, "referral_qualified", plat.XPReferrerPoints,
		map[string]any{"role": "referrer", "referee_id": refereeID},
		"referral:"+refereeID+":referrer")
}

// handleNFTRefresh no longer grants a personal NFT entitlement — product access
// is organization-only. It still verifies wallet ownership (so NFT-held XP keeps
// accruing) and returns the organization-derived entitlement for compatibility.
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
	plat := s.platform.Snapshot()
	// XP driver: holding the NFT earns XP once per month (best-effort).
	month := time.Now().UTC().Format("200601")
	_, _ = s.store.AwardXPOnce(c, u.ID, "nft_held", plat.XPNFTHeld,
		map[string]any{"month": month}, "nft_held:"+u.ID+":"+month)

	resp, err := s.orgEntitlementResponse(c, u.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load subscription")
		return
	}
	resp["nft_verified"] = true
	resp["nft_grant_retired"] = true
	ok(c, http.StatusOK, resp)
}
