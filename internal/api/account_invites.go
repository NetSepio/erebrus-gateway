package api

import (
	"errors"
	"net/http"
	"strings"

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
	email := verifiedEmail(u)
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

func (s *Server) handleGetAccountOrgInvite(c *gin.Context) {
	u, err := s.store.GetUser(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load user")
		return
	}
	inv, err := s.store.GetUserOrgInvite(c, u.ID, c.Param("orgId"), verifiedEmail(u))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "invite not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load invite")
		return
	}
	ok(c, http.StatusOK, inv)
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
	email := verifiedEmail(u)
	orgID := c.Param("orgId")
	ctx, err := s.store.InviteNotificationContextForUser(c, u.ID, orgID, email)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "invite not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load invite")
		return
	}
	if err := s.store.AcceptUserOrgInvite(c, u.ID, orgID, email); err != nil {
		fail(c, http.StatusInternalServerError, "failed to accept invite")
		return
	}
	s.logActivity(c, u.ID, "org.invite.accept", orgID)
	s.sendOrgInviteOutcomeEmails(c, ctx, true)
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
	email := verifiedEmail(u)
	orgID := c.Param("orgId")
	ctx, err := s.store.InviteNotificationContextForUser(c, u.ID, orgID, email)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "invite not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load invite")
		return
	}
	if err := s.store.DeclineUserOrgInvite(c, u.ID, orgID, email); err != nil {
		fail(c, http.StatusInternalServerError, "failed to decline invite")
		return
	}
	s.logActivity(c, u.ID, "org.invite.decline", orgID)
	s.sendOrgInviteOutcomeEmails(c, ctx, false)
	c.Status(http.StatusNoContent)
}

func verifiedEmail(u *store.User) string {
	if u != nil && u.EmailVerified {
		return strings.TrimSpace(u.Email)
	}
	return ""
}

func (s *Server) sendOrgInviteOutcomeEmails(c *gin.Context, ctx *store.InviteNotificationContext, accepted bool) {
	if !s.mailer.Enabled() || ctx == nil {
		return
	}
	role := humanRole(ctx.Role)
	workspaceURL := strings.TrimRight(s.cfg.ErebrusPublicBaseURL, "/") + "/workspace/" + ctx.OrgID
	invitee := ctx.InviteeName
	if invitee == "" {
		invitee = "A user"
	}

	if ctx.InviteeEmail != "" {
		if accepted {
			_ = s.mailer.SendOrgInviteAccepted(c, ctx.InviteeEmail, ctx.OrgDisplayName, invitee, role, workspaceURL, false)
		} else {
			_ = s.mailer.SendOrgInviteDeclined(c, ctx.InviteeEmail, ctx.OrgDisplayName, invitee, role, false)
		}
	}

	notifyInviter := ctx.InviterEmail != "" && !strings.EqualFold(ctx.InviterEmail, ctx.InviteeEmail)
	if notifyInviter {
		if accepted {
			_ = s.mailer.SendOrgInviteAccepted(c, ctx.InviterEmail, ctx.OrgDisplayName, invitee, role, workspaceURL, true)
		} else {
			_ = s.mailer.SendOrgInviteDeclined(c, ctx.InviterEmail, ctx.OrgDisplayName, invitee, role, true)
		}
	}

	notifyOwner := ctx.OwnerEmail != "" &&
		!strings.EqualFold(ctx.OwnerEmail, ctx.InviteeEmail) &&
		!strings.EqualFold(ctx.OwnerEmail, ctx.InviterEmail)
	if notifyOwner {
		if accepted {
			_ = s.mailer.SendOrgInviteAccepted(c, ctx.OwnerEmail, ctx.OrgDisplayName, invitee, role, workspaceURL, true)
		} else {
			_ = s.mailer.SendOrgInviteDeclined(c, ctx.OwnerEmail, ctx.OrgDisplayName, invitee, role, true)
		}
	}
}

func humanRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case store.OrgRoleAdmin:
		return "Admin"
	case store.OrgRoleNodeOperator:
		return "Node operator"
	case store.OrgRoleMember:
		return "Member"
	case store.OrgRoleViewer:
		return "Viewer"
	default:
		if role == "" {
			return "Member"
		}
		return role
	}
}