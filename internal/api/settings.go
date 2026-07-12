package api

import (
	"net/http"

	"github.com/NetSepio/gateway/internal/socialverify"
	"github.com/gin-gonic/gin"
)

// handleAdminListSettings returns all platform_settings rows.
func (s *Server) handleAdminListSettings(c *gin.Context) {
	settings, err := s.store.ListPlatformSettings(c)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load settings")
		return
	}
	ok(c, http.StatusOK, gin.H{"settings": settings})
}

type patchSettingsReq struct {
	Settings map[string]string `json:"settings"`
}

// handleAdminPatchSettings updates known platform_settings keys and reloads
// the in-memory copy (tier thresholds, rate limits, XP weights, etc.).
func (s *Server) handleAdminPatchSettings(c *gin.Context) {
	var req patchSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Settings) == 0 {
		fail(c, http.StatusBadRequest, "settings object required")
		return
	}

	updated, err := s.store.UpdatePlatformSettings(c, req.Settings)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	s.platform.Replace(updated)
	plat := s.platform.Snapshot()
	s.tokens.Reconfigure(plat.PasetoSignedBy, plat.PasetoExpiration)
	if v, ok := req.Settings["x_api_base_url"]; ok && v != "" {
		s.xverify = socialverify.NewXVerifier(v)
	}
	ok(c, http.StatusOK, gin.H{"status": "updated"})
}
