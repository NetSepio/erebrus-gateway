package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// handleReferralsMe returns the caller's referral code, who referred them, and
// their recent referees. The code is allocated lazily on first access.
func (s *Server) handleReferralsMe(c *gin.Context) {
	sum, err := s.store.ReferralSummary(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load referrals")
		return
	}
	// Truncate wallets in the response (the referrer never sees full referee addrs).
	sum.ReferredBy = truncWallet(sum.ReferredBy)
	for i := range sum.Recent {
		sum.Recent[i].Wallet = truncWallet(sum.Recent[i].Wallet)
	}
	ok(c, http.StatusOK, sum)
}

// truncWallet shortens an address to first6…last4 for display, leaving short or
// empty values unchanged.
func truncWallet(w string) string {
	if len(w) <= 12 {
		return w
	}
	return w[:6] + "…" + w[len(w)-4:]
}

// bindReferralCode best-effort binds a referral code during login/signup:
// resolves the code, sets the caller's referrer (one-shot, self-blocked) and,
// when the referee has already started their qualifying trial, fires the
// (idempotent) XP awards immediately — otherwise the trial-start path fires
// them. Invalid or already-bound codes are ignored so login never fails.
func (s *Server) bindReferralCode(c *gin.Context, userID, code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return
	}
	referrerID, err := s.store.UserIDByReferralCode(c, code)
	if err != nil {
		return
	}
	bound, err := s.store.BindReferrer(c, userID, referrerID)
	if err != nil || !bound {
		return
	}
	if qualified, err := s.store.HasQualifiedTrial(c, userID); err == nil && qualified {
		s.awardReferralXP(c, userID)
	}
}

// handleReferralRedeem lets a signed-in user apply an invite code after signup:
// POST /api/v2/referrals/redeem. One referrer per account, ever; when the
// caller already qualified (their trial has started) both XP awards fire
// immediately, dedup-keyed so nothing can double-pay.
func (s *Server) handleReferralRedeem(c *gin.Context) {
	var req struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Code) == "" {
		fail(c, http.StatusBadRequest, "code is required")
		return
	}
	uid := userID(c)
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	referrerID, err := s.store.UserIDByReferralCode(c, code)
	if err != nil {
		fail(c, http.StatusNotFound, "invite code not found")
		return
	}
	if referrerID == uid {
		fail(c, http.StatusBadRequest, "you can't redeem your own invite code")
		return
	}
	bound, err := s.store.BindReferrer(c, uid, referrerID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to apply invite code")
		return
	}
	if !bound {
		fail(c, http.StatusConflict, "an invite code is already applied to this account")
		return
	}
	if qualified, err := s.store.HasQualifiedTrial(c, uid); err == nil && qualified {
		s.awardReferralXP(c, uid)
	}

	sum, err := s.store.ReferralSummary(c, uid)
	if err != nil {
		ok(c, http.StatusOK, gin.H{"bound": true})
		return
	}
	sum.ReferredBy = truncWallet(sum.ReferredBy)
	for i := range sum.Recent {
		sum.Recent[i].Wallet = truncWallet(sum.Recent[i].Wallet)
	}
	ok(c, http.StatusOK, sum)
}
