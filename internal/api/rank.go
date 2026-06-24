package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// tierNames label the §5b tiers; index = tier. Falls back to "Tier N" if the
// configured thresholds change the count.
var tierNames = []string{"Newcomer", "Connected", "Contributor", "Guardian", "Architect"}

func tierName(tier int) string {
	if tier >= 0 && tier < len(tierNames) {
		return tierNames[tier]
	}
	return "Tier " + strconv.Itoa(tier)
}

// handleRankMe returns the caller's XP standing: lifetime earned, claimed, the
// claimable balance, current tier, the XP needed for the next tier, and a
// per-driver breakdown.
func (s *Server) handleRankMe(c *gin.Context) {
	uid := userID(c)
	earned, claimed, tier, err := s.store.UserXP(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load rank")
		return
	}
	breakdown, err := s.store.XPBreakdown(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load rank")
		return
	}

	resp := gin.H{
		"xp_earned":         earned,
		"xp_claimed":        claimed,
		"xp_claimable":      max64(earned-claimed, 0),
		"tier":              tier,
		"tier_name":         tierName(tier),
		"breakdown_by_kind": breakdown,
	}
	// next_tier_at = the first threshold strictly above the current lifetime XP.
	for _, t := range s.store.TierThresholds() {
		if t > earned {
			resp["next_tier_at"] = t
			break
		}
	}
	ok(c, http.StatusOK, resp)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
