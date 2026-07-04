package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// orgPaidSeat ensures the caller owns, or holds a paid seat in, the path :id org.
// AdGuard admin credentials are revealed only to these "managers".
func (s *Server) orgPaidSeat(c *gin.Context) bool {
	ok, err := s.store.UserHasOrgSeat(c, c.Param("id"), userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "seat check failed")
		return false
	}
	if !ok {
		fail(c, http.StatusForbidden, "a paid seat in this org is required")
		return false
	}
	return true
}

// orgNode resolves the path :nodeId and verifies it belongs to the path :id org.
func (s *Server) orgNode(c *gin.Context) (*store.Node, bool) {
	node, err := s.store.GetNode(c, c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && node.OrgID != c.Param("id")) {
		fail(c, http.StatusNotFound, "node not found in this org")
		return nil, false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return nil, false
	}
	return node, true
}

// handleNodeReportFirewallCredentials ingests a Shield node's AdGuard admin
// credentials (node PASETO). The password is encrypted before storage.
func (s *Server) handleNodeReportFirewallCredentials(c *gin.Context) {
	claims, err := s.tokens.Verify(bearer(c))
	peerID := nodeTokenPeerID(claims)
	if err != nil || claims.Role != token.RoleNode || peerID == "" {
		fail(c, http.StatusUnauthorized, "valid node token required")
		return
	}
	if param := c.Param("nodeId"); param != "" && param != peerID {
		if resolved, rerr := s.store.ResolvePeerID(c, param); rerr != nil || resolved != peerID {
			fail(c, http.StatusForbidden, "node id mismatch")
			return
		}
	}
	if !s.crypt.Enabled() {
		fail(c, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	var req struct {
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
		AdminURL      string `json:"admin_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AdminPassword) == "" {
		fail(c, http.StatusBadRequest, "admin_password is required")
		return
	}
	sealed, err := s.crypt.Seal(req.AdminPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to encrypt credential")
		return
	}
	if err := s.store.UpsertFirewallCredential(c, peerID, strings.TrimSpace(req.AdminUser), sealed, strings.TrimSpace(req.AdminURL)); err != nil {
		fail(c, http.StatusInternalServerError, "failed to store credential")
		return
	}
	c.Status(http.StatusNoContent)
}

// handleGetFirewallCredentials reveals a node's AdGuard admin credential to an
// org's paid seats (audit-logged).
func (s *Server) handleGetFirewallCredentials(c *gin.Context) {
	if !s.orgPaidSeat(c) {
		return
	}
	node, found := s.orgNode(c)
	if !found {
		return
	}
	cred, err := s.store.GetFirewallCredential(c, node.PeerID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "no firewall credential reported for this node yet")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load credential")
		return
	}
	password, err := s.crypt.Open(cred.Secret)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to decrypt credential")
		return
	}
	s.logActivity(c, userID(c), "firewall.credentials.view", node.PeerID)
	ok(c, http.StatusOK, gin.H{
		"node_id":        node.PeerID,
		"admin_user":     cred.AdminUser,
		"admin_password": password,
		"admin_url":      cred.AdminURL,
		"updated_at":     cred.UpdatedAt,
	})
}

// handleUpdateFirewallCredentials sets a new admin password (paid seat), stores
// it encrypted, and pushes it to the node to apply on AdGuard.
func (s *Server) handleUpdateFirewallCredentials(c *gin.Context) {
	if !s.orgPaidSeat(c) {
		return
	}
	node, found := s.orgNode(c)
	if !found {
		return
	}
	if !s.crypt.Enabled() {
		fail(c, http.StatusServiceUnavailable, "credential encryption not configured")
		return
	}
	var req struct {
		AdminUser     string `json:"admin_user"`
		AdminPassword string `json:"admin_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AdminPassword) == "" {
		fail(c, http.StatusBadRequest, "admin_password is required")
		return
	}
	// Keep the existing user/url unless overridden.
	adminUser := strings.TrimSpace(req.AdminUser)
	adminURL := ""
	if cur, cerr := s.store.GetFirewallCredential(c, node.PeerID); cerr == nil {
		if adminUser == "" {
			adminUser = cur.AdminUser
		}
		adminURL = cur.AdminURL
	}
	sealed, err := s.crypt.Seal(req.AdminPassword)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to encrypt credential")
		return
	}
	if err := s.store.UpsertFirewallCredential(c, node.PeerID, adminUser, sealed, adminURL); err != nil {
		fail(c, http.StatusInternalServerError, "failed to store credential")
		return
	}
	// Best-effort push to the node to apply on AdGuard (node may be offline).
	args, _ := json.Marshal(map[string]string{"admin_user": adminUser, "admin_password": req.AdminPassword})
	notified := s.hub.SendCommand(node.PeerID, "set_firewall_credentials", args, uuid.NewString())
	s.logActivity(c, userID(c), "firewall.credentials.update", node.PeerID)
	ok(c, http.StatusOK, gin.H{"node_id": node.PeerID, "admin_user": adminUser, "node_notified": notified})
}
