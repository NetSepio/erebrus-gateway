package api

import (
	"net/http"
	"strings"

	"github.com/NetSepio/gateway/internal/gw/token"
	"github.com/gin-gonic/gin"
)

// gin context keys.
const (
	ctxUserID = "user_id"
	ctxWallet = "wallet"
	ctxRole   = "role"
	ctxOrgID  = "org_id"
)

// authUser validates a user/admin PASETO bearer token and stores identity in
// the gin context.
func (s *Server) authUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		tok := bearer(c)
		if tok == "" {
			fail(c, http.StatusUnauthorized, "authorization bearer token required")
			return
		}
		claims, err := s.tokens.Verify(tok)
		if err != nil {
			fail(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		if claims.Role == token.RoleNode {
			fail(c, http.StatusForbidden, "node token not valid for user routes")
			return
		}
		if claims.UserID == "" {
			fail(c, http.StatusUnauthorized, "token missing subject")
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxWallet, claims.Wallet)
		c.Set(ctxRole, claims.Role)
		c.Next()
	}
}

// requireAdmin gates admin routes; must run after authUser.
func (s *Server) requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(ctxRole) != token.RoleAdmin {
			fail(c, http.StatusForbidden, "admin role required")
			return
		}
		c.Next()
	}
}

// authAPIKey validates an org API key (X-Api-Key), sets the org in context, and
// meters the call. Used for programmatic, org-scoped routes.
func (s *Server) authAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("X-Api-Key"))
		if key == "" {
			fail(c, http.StatusUnauthorized, "X-Api-Key header required")
			return
		}
		orgID, err := s.store.LookupAPIKey(c, key)
		if err != nil {
			fail(c, http.StatusUnauthorized, "invalid or revoked API key")
			return
		}
		c.Set(ctxOrgID, orgID)
		s.store.IncrAPICall(c, orgID) // best-effort metering
		c.Next()
	}
}

func orgID(c *gin.Context) string { return c.GetString(ctxOrgID) }

func bearer(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

func userID(c *gin.Context) string { return c.GetString(ctxUserID) }
