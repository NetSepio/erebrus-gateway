package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// ── membership guards ────────────────────────────────

// orgMember ensures the caller is a member of the path :id org. Returns the
// caller's role; on failure it writes the response and returns ok=false.
func (s *Server) orgMember(c *gin.Context) (role string, ok bool) {
	role, err := s.store.MemberRole(c, c.Param("id"), userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusForbidden, "not a member of this org")
		return "", false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "membership check failed")
		return "", false
	}
	return role, true
}

func (s *Server) orgOwner(c *gin.Context) bool {
	role, ok := s.orgMember(c)
	if !ok {
		return false
	}
	if role != "owner" {
		fail(c, http.StatusForbidden, "owner role required")
		return false
	}
	return true
}

// ── org management (user-authed) ─────────────────────

func (s *Server) handleCreateOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		fail(c, http.StatusBadRequest, "name is required")
		return
	}
	org, err := s.store.CreateOrg(c, req.Name, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create org")
		return
	}
	ok(c, http.StatusCreated, org)
}

func (s *Server) handleListOrgs(c *gin.Context) {
	orgs, err := s.store.ListOrgsForUser(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list orgs")
		return
	}
	ok(c, http.StatusOK, orgs)
}

func (s *Server) handleGetOrg(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	org, err := s.store.GetOrg(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, "org not found")
		return
	}
	ok(c, http.StatusOK, org)
}

func (s *Server) handleListMembers(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	members, err := s.store.ListMembers(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list members")
		return
	}
	ok(c, http.StatusOK, members)
}

func (s *Server) handleAddMember(c *gin.Context) {
	if !s.orgOwner(c) {
		return
	}
	var req struct {
		WalletAddress string `json:"wallet_address"`
		Chain         string `json:"chain"`
		Role          string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.WalletAddress == "" {
		fail(c, http.StatusBadRequest, "wallet_address is required")
		return
	}
	role := req.Role
	if role != "owner" {
		role = "member"
	}
	u, err := s.store.UpsertUserByWallet(c, req.WalletAddress, req.Chain, "")
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve user")
		return
	}
	if err := s.store.AddMember(c, c.Param("id"), u.ID, role); err != nil {
		fail(c, http.StatusInternalServerError, "failed to add member")
		return
	}
	ok(c, http.StatusCreated, gin.H{"user_id": u.ID, "wallet_address": u.WalletAddress, "role": role})
}

// ── API keys ─────────────────────────────────────────

func (s *Server) handleListAPIKeys(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	keys, err := s.store.ListAPIKeys(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list keys")
		return
	}
	ok(c, http.StatusOK, keys)
}

func (s *Server) handleCreateAPIKey(c *gin.Context) {
	if !s.orgOwner(c) {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&req)
	secret, key, err := s.store.CreateAPIKey(c, c.Param("id"), req.Name)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create key")
		return
	}
	// The secret is returned exactly once.
	ok(c, http.StatusCreated, gin.H{"api_key": secret, "key": key})
}

func (s *Server) handleRevokeAPIKey(c *gin.Context) {
	if !s.orgOwner(c) {
		return
	}
	err := s.store.RevokeAPIKey(c, c.Param("id"), c.Param("keyId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "key not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to revoke key")
		return
	}
	c.Status(http.StatusNoContent)
}

// ── org usage & clients (user-authed) ────────────────

func (s *Server) handleOrgUsage(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	ok(c, http.StatusOK, s.orgUsagePayload(c, c.Param("id")))
}

func (s *Server) handleOrgClients(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	clients, err := s.store.OrgClients(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list clients")
		return
	}
	ok(c, http.StatusOK, clients)
}

// ── org programmatic access (X-Api-Key) ──────────────

func (s *Server) handleOrgProvisionClient(c *gin.Context) {
	oid := orgID(c)
	org, err := s.store.GetOrg(c, oid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve org")
		return
	}
	var req provisionReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.NodeID == "" || req.WGPublicKey == "" {
		fail(c, http.StatusBadRequest, "name, node_id and wg_public_key are required")
		return
	}
	// Org access (a valid API key) is the entitlement; clients are owned by the
	// org and attributed to its owner user.
	s.doProvision(c, org.OwnerUserID, oid, req.NodeID, req.Name, req.WGPublicKey, req.WGPresharedKey)
}

func (s *Server) handleOrgListClients(c *gin.Context) {
	clients, err := s.store.OrgClients(c, orgID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list clients")
		return
	}
	ok(c, http.StatusOK, clients)
}

func (s *Server) handleOrgSelfUsage(c *gin.Context) {
	ok(c, http.StatusOK, s.orgUsagePayload(c, orgID(c)))
}

// ── admin org views ──────────────────────────────────

func (s *Server) handleAdminOrgs(c *gin.Context) {
	limit, offset := pagination(c)
	orgs, err := s.store.ListOrgs(c, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list orgs")
		return
	}
	ok(c, http.StatusOK, gin.H{"orgs": orgs, "limit": limit, "offset": offset})
}

func (s *Server) handleAdminOrgUsage(c *gin.Context) {
	ok(c, http.StatusOK, s.orgUsagePayload(c, c.Param("id")))
}

// orgUsagePayload assembles the usage rollup for an org over a window (default
// 30 days; override with ?days=N).
func (s *Server) orgUsagePayload(c *gin.Context, oid string) gin.H {
	days := 30
	if v, err := strconv.Atoi(c.Query("days")); err == nil && v > 0 && v <= 365 {
		days = v
	}
	since := time.Now().AddDate(0, 0, -days)
	rx, tx, _ := s.store.OrgBandwidth(c, oid, since)
	calls, _ := s.store.OrgAPICalls(c, oid, since)
	clients, _ := s.store.OrgClients(c, oid)
	return gin.H{
		"org_id":          oid,
		"window_days":     days,
		"clients":         len(clients),
		"api_calls":       calls,
		"bandwidth_rx":    rx,
		"bandwidth_tx":    tx,
		"bandwidth_total": rx + tx,
	}
}
