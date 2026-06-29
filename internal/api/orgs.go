package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// orgMember ensures the caller is a member of the path :id org.
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
	if role != store.OrgRoleOwner {
		fail(c, http.StatusForbidden, "owner role required")
		return false
	}
	return true
}

func (s *Server) orgPrivileged(c *gin.Context) (role string, ok bool) {
	role, ok = s.orgMember(c)
	if !ok {
		return "", false
	}
	if !store.IsOrgPrivileged(role) {
		fail(c, http.StatusForbidden, "owner or admin role required")
		return "", false
	}
	return role, true
}

// ── org management (user-authed) ─────────────────────

func (s *Server) handleCreateOrg(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		fail(c, http.StatusBadRequest, "name is required")
		return
	}
	org, err := s.store.CreateOrgForUser(c, userID(c), store.CreateOrgInput{
		Name: req.Name, Slug: req.Slug,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create org")
		return
	}
	ok(c, http.StatusCreated, orgResponse(org, true)) // creator is owner
}

func (s *Server) handleListOrgs(c *gin.Context) {
	orgs, err := s.store.ListOrgsForUser(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list orgs")
		return
	}
	out := make([]gin.H, 0, len(orgs))
	for i := range orgs {
		out = append(out, orgResponse(&orgs[i], store.IsOrgPrivileged(orgs[i].Role)))
	}
	ok(c, http.StatusOK, out)
}

func (s *Server) handleGetOrg(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	org, err := s.store.GetOrg(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusNotFound, "org not found")
		return
	}
	org.Role = role
	ok(c, http.StatusOK, orgResponse(org, store.IsOrgPrivileged(role)))
}

func (s *Server) handlePatchOrg(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req struct {
		Name                 *string `json:"name"`
		Slug                 *string `json:"slug"`
		BillingStatus        *string `json:"billing_status"`
		PublicProfileEnabled *bool   `json:"public_profile_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	org, err := s.store.UpdateOrg(c, c.Param("id"), store.UpdateOrgInput{
		Name: req.Name, Slug: req.Slug,
		BillingStatus: req.BillingStatus, PublicProfileEnabled: req.PublicProfileEnabled,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update org")
		return
	}
	role, _ := s.store.MemberRole(c, org.ID, userID(c))
	org.Role = role
	ok(c, http.StatusOK, orgResponse(org, store.IsOrgPrivileged(role)))
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

func (s *Server) handleInviteMember(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req struct {
		WalletAddress string `json:"wallet_address"`
		Chain         string `json:"chain"`
		Role          string `json:"role"`
		SeatTier      string `json:"seat_tier"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.WalletAddress == "" {
		fail(c, http.StatusBadRequest, "wallet_address is required")
		return
	}
	role := normalizeMemberRole(req.Role)
	if role == store.OrgRoleOwner {
		fail(c, http.StatusBadRequest, "use transfer-ownership to change owner")
		return
	}
	u, err := s.store.UpsertUserByWallet(c, req.WalletAddress, req.Chain, "")
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve user")
		return
	}
	member, err := s.store.InviteMember(c, c.Param("id"), u.ID, role, req.SeatTier)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to invite member")
		return
	}
	member.WalletAddress = u.WalletAddress
	ok(c, http.StatusCreated, member)
}

func (s *Server) handleAddMember(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
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
	role := normalizeMemberRole(req.Role)
	if role == store.OrgRoleOwner {
		fail(c, http.StatusBadRequest, "use transfer-ownership to change owner")
		return
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

func (s *Server) handlePatchMember(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req struct {
		Role     string `json:"role"`
		SeatTier string `json:"seat_tier"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Role == "" && req.SeatTier == "" {
		fail(c, http.StatusBadRequest, "role or seat_tier is required")
		return
	}
	if req.Role == store.OrgRoleOwner {
		fail(c, http.StatusBadRequest, "use transfer-ownership to change owner")
		return
	}
	orgID := c.Param("id")
	memberKey := c.Param("memberId")
	if member, err := s.store.PatchMember(c, orgID, memberKey, req.Role, req.SeatTier); err == nil {
		ok(c, http.StatusOK, member)
		return
	}
	if req.SeatTier != "" {
		fail(c, http.StatusBadRequest, "seat_tier updates require member id")
		return
	}
	role := normalizeMemberRole(req.Role)
	curRole, err := s.store.MemberRole(c, orgID, memberKey)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "member not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "membership check failed")
		return
	}
	if curRole == store.OrgRoleOwner {
		fail(c, http.StatusForbidden, "cannot change owner role here")
		return
	}
	if err := s.store.AddMember(c, orgID, memberKey, role); err != nil {
		fail(c, http.StatusInternalServerError, "failed to update member")
		return
	}
	ok(c, http.StatusOK, gin.H{"user_id": memberKey, "role": role})
}

func (s *Server) handleRemoveMember(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	orgID := c.Param("id")
	memberKey := c.Param("memberId")
	if err := s.store.RemoveMemberByID(c, orgID, memberKey); err == nil {
		c.Status(http.StatusNoContent)
		return
	}
	if err := s.store.RemoveMember(c, orgID, memberKey); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "member not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to remove member")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleTransferOwnership(c *gin.Context) {
	if !s.orgOwner(c) {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
		fail(c, http.StatusBadRequest, "user_id is required")
		return
	}
	if err := s.store.TransferOrgOwnership(c, c.Param("id"), userID(c), req.UserID); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusBadRequest, "target must be an existing admin member")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to transfer ownership")
		return
	}
	ok(c, http.StatusOK, gin.H{"org_id": c.Param("id"), "owner_user_id": req.UserID})
}

func (s *Server) handleOrgNodes(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	nodes, err := s.store.NodesByOrg(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]nodeOperatorView, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, s.buildNodeOperatorView(c, n, role))
	}
	ok(c, http.StatusOK, out)
}

// ── API keys ─────────────────────────────────────────

func (s *Server) handleListAPIKeys(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
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
	role, ok := s.orgMember(c)
	if !ok {
		return
	}
	okJSON(c, http.StatusOK, s.orgUsagePayload(c, c.Param("id"), role))
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
	node, err := s.store.GetNode(c, req.NodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	if node.OrgID != oid {
		fail(c, http.StatusForbidden, "node does not belong to this org")
		return
	}
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
	okJSON(c, http.StatusOK, s.orgUsagePayload(c, orgID(c), store.OrgRoleAdmin))
}

// ── admin org views ──────────────────────────────────

func (s *Server) handleAdminOrgs(c *gin.Context) {
	limit, offset := pagination(c)
	orgs, err := s.store.ListOrgs(c, limit, offset)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list orgs")
		return
	}
	out := make([]gin.H, 0, len(orgs))
	for i := range orgs {
		out = append(out, orgResponse(&orgs[i], false))
	}
	ok(c, http.StatusOK, gin.H{"orgs": out, "limit": limit, "offset": offset})
}

type patchAdminOrgReq struct {
	VerificationStatus *string `json:"verification_status"`
	Plan               *string `json:"plan"`
}

func (s *Server) handleAdminPatchOrg(c *gin.Context) {
	var req patchAdminOrgReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.VerificationStatus == nil && req.Plan == nil {
		fail(c, http.StatusBadRequest, "verification_status or plan is required")
		return
	}
	oid := c.Param("id")
	if req.VerificationStatus != nil {
		if err := s.store.SetOrgVerificationStatus(c, oid, *req.VerificationStatus); errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "org not found")
			return
		} else if err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Plan != nil {
		enabled, region := s.cfg.ProvisioningConfig()
		if _, err := s.store.SetOrgPlanAndProvision(c, oid, *req.Plan, store.ProvisioningConfig{
			Enabled: enabled, DefaultRegion: region,
		}); errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "org not found")
			return
		} else if err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
	}
	org, err := s.store.GetOrg(c, oid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load org")
		return
	}
	ok(c, http.StatusOK, orgResponse(org, true))
}

func (s *Server) handleAdminOrgUsage(c *gin.Context) {
	okJSON(c, http.StatusOK, s.orgUsagePayload(c, c.Param("id"), store.OrgRoleOwner))
}

func (s *Server) orgUsagePayload(c *gin.Context, oid, role string) gin.H {
	days := 30
	if v, err := strconv.Atoi(c.Query("days")); err == nil && v > 0 && v <= 365 {
		days = v
	}
	since := time.Now().AddDate(0, 0, -days)
	rx, tx, _ := s.store.OrgBandwidth(c, oid, since)
	calls, _ := s.store.OrgAPICalls(c, oid, since)
	clients, _ := s.store.OrgClients(c, oid)
	out := gin.H{
		"window_days":     days,
		"clients":         len(clients),
		"api_calls":       calls,
		"bandwidth_rx":    rx,
		"bandwidth_tx":    tx,
		"bandwidth_total": rx + tx,
	}
	if store.IsOrgPrivileged(role) {
		out["org_id"] = oid
	}
	return out
}

func (s *Server) handleGetOrgEntitlements(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	ent, err := s.store.GetOrgEntitlements(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "entitlements not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load entitlements")
		return
	}
	ok(c, http.StatusOK, entitlementResponse(ent))
}

func (s *Server) handleGetOrgProfile(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	profile, err := s.store.GetOrgProfile(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load profile")
		return
	}
	ok(c, http.StatusOK, orgProfileResponse(profile))
}

func (s *Server) handlePatchOrgProfile(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req store.UpdateOrgProfileInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	profile, err := s.store.UpdateOrgProfile(c, c.Param("id"), req)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update profile")
		return
	}
	ok(c, http.StatusOK, orgProfileResponse(profile))
}

func (s *Server) handleAssignSeat(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req struct {
		UserID   string `json:"user_id"`
		SeatTier string `json:"seat_tier"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" || req.SeatTier == "" {
		fail(c, http.StatusBadRequest, "user_id and seat_tier are required")
		return
	}
	if err := s.store.AssignSeat(c, c.Param("id"), req.UserID, req.SeatTier); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "member not found")
		return
	} else if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, http.StatusOK, gin.H{"user_id": req.UserID, "seat_tier": req.SeatTier})
}

func (s *Server) handleRevokeSeat(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.UserID == "" {
		fail(c, http.StatusBadRequest, "user_id is required")
		return
	}
	if err := s.store.RevokeSeat(c, c.Param("id"), req.UserID); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "member not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to revoke seat")
		return
	}
	ok(c, http.StatusOK, gin.H{"user_id": req.UserID, "seat_tier": store.SeatTierFree})
}

func (s *Server) handleListSeats(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	members, err := s.store.ListMembers(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list seats")
		return
	}
	ent, _ := s.store.GetOrgEntitlements(c, c.Param("id"))
	used, _ := s.store.CountPaidSeatsUsed(c, c.Param("id"))
	out := gin.H{"members": members, "paid_seats_used": used}
	if ent != nil {
		out["paid_seats_included"] = ent.PaidSeatsIncluded
	}
	ok(c, http.StatusOK, out)
}

func normalizeMemberRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case store.OrgRoleAdmin:
		return store.OrgRoleAdmin
	case store.OrgRoleOwner:
		return store.OrgRoleOwner
	case store.OrgRoleNodeOperator:
		return store.OrgRoleNodeOperator
	case store.OrgRoleViewer:
		return store.OrgRoleViewer
	default:
		return store.OrgRoleMember
	}
}

func okJSON(c *gin.Context, code int, payload gin.H) {
	ok(c, code, payload)
}