package api

import (
	"net/http"

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
