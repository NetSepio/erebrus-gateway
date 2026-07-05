package api

import (
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleListAccountOrgInvites(c *gin.Context) {
	u, err := s.store.GetUser(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	email := ""
	if u.EmailVerified {
		email = u.Email
	}
	invites, err := s.store.ListUserOrgInvites(c, u.ID, email)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list invites")
		return
	}
	if invites == nil {
		invites = []store.UserOrgInvite{}
	}
	ok(c, http.StatusOK, invites)
}

func (s *Server) handleAcceptAccountOrgInvite(c *gin.Context) {
	u, err := s.store.GetUser(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	email := ""
	if u.EmailVerified {
		email = u.Email
	}
	orgID := c.Param("orgId")
	if err := s.store.AcceptUserOrgInvite(c, u.ID, orgID, email); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "invite not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to accept invite")
		return
	}
	s.logActivity(c, u.ID, "org.invite.accept", orgID)
	ok(c, http.StatusOK, gin.H{"status": "active", "org_id": orgID})
}

func (s *Server) handleDeclineAccountOrgInvite(c *gin.Context) {
	u, err := s.store.GetUser(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	email := ""
	if u.EmailVerified {
		email = u.Email
	}
	orgID := c.Param("orgId")
	if err := s.store.DeclineUserOrgInvite(c, u.ID, orgID, email); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "invite not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to decline invite")
		return
	}
	s.logActivity(c, u.ID, "org.invite.decline", orgID)
	c.Status(http.StatusNoContent)
}