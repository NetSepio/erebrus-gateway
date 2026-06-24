package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/NetSepio/gateway/internal/gw/socialverify"
	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/gin-gonic/gin"
)

// telegramAuthMaxAge bounds Login Widget replay.
const telegramAuthMaxAge = 24 * time.Hour

// handleSocialAccounts lists the caller's verified social accounts.
func (s *Server) handleSocialAccounts(c *gin.Context) {
	accts, err := s.store.ListSocialAccounts(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load social accounts")
		return
	}
	ok(c, http.StatusOK, accts)
}

// handleVerifyTelegram links a Telegram account from a Login Widget payload.
func (s *Server) handleVerifyTelegram(c *gin.Context) {
	if s.cfg.TelegramBotToken == "" {
		fail(c, http.StatusServiceUnavailable, "telegram verification not configured")
		return
	}
	var fields map[string]string
	if err := c.ShouldBindJSON(&fields); err != nil || len(fields) == 0 {
		fail(c, http.StatusBadRequest, "telegram widget payload required")
		return
	}
	pid, handle, err := socialverify.VerifyTelegram(s.cfg.TelegramBotToken, fields, telegramAuthMaxAge)
	if err != nil {
		fail(c, http.StatusUnauthorized, "telegram verification failed")
		return
	}
	s.linkSocial(c, "telegram", pid, handle)
}

type xVerifyReq struct {
	AccessToken string `json:"access_token"`
}

// handleVerifyX links an X account from the user's OAuth2 access token.
func (s *Server) handleVerifyX(c *gin.Context) {
	var req xVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil || req.AccessToken == "" {
		fail(c, http.StatusBadRequest, "access_token required")
		return
	}
	pid, handle, err := s.xverify.Verify(c, req.AccessToken)
	if err != nil {
		fail(c, http.StatusUnauthorized, "x verification failed")
		return
	}
	s.linkSocial(c, "x", pid, handle)
}

// linkSocial records the verified account and awards social_verified XP once per
// provider, then writes the response.
func (s *Server) linkSocial(c *gin.Context, provider, providerID, handle string) {
	uid := userID(c)
	created, err := s.store.LinkSocialAccount(c, uid, provider, providerID, handle)
	if errors.Is(err, store.ErrSocialTaken) {
		fail(c, http.StatusConflict, "already linked to another account")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to link account")
		return
	}
	if created {
		_, _ = s.store.AwardXPOnce(c, uid, "social_verified", s.cfg.XPSocialVerified,
			map[string]any{"provider": provider}, "social_verified:"+uid+":"+provider)
	}
	ok(c, http.StatusOK, gin.H{"provider": provider, "handle": handle, "newly_linked": created})
}
