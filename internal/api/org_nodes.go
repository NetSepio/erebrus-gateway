package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleCreateNodeRegistrationToken(c *gin.Context) {
	if _, ok := s.orgCanManageNodes(c); !ok {
		return
	}
	var req struct {
		PeerID    string    `json:"peer_id"`
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
		TTLHours  int       `json:"ttl_hours"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{store.TokenScopeNodeRegistration}
	}
	expiresAt := req.ExpiresAt
	if expiresAt.IsZero() {
		hours := req.TTLHours
		if hours <= 0 {
			hours = 24
		}
		expiresAt = time.Now().Add(time.Duration(hours) * time.Hour)
	}
	plain, tok, err := s.store.CreateNodeRegistrationToken(c, c.Param("id"), userID(c), strings.TrimSpace(req.PeerID), scopes, expiresAt)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, http.StatusCreated, gin.H{
		"token":      plain,
		"token_meta": tok,
	})
}

func (s *Server) handleListOrgNodes(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	nodes, err := s.store.ListOrgNodes(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list org nodes")
		return
	}
	ok(c, http.StatusOK, nodes)
}

func (s *Server) handleGetOrgNode(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	node, err := s.store.GetOrgNode(c, c.Param("id"), c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load node")
		return
	}
	ok(c, http.StatusOK, node)
}

func (s *Server) handlePatchOrgNode(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return
	}
	var req store.UpdateOrgNodeInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	node, err := s.store.UpdateOrgNode(c, c.Param("id"), c.Param("nodeId"), req)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update node")
		return
	}
	ok(c, http.StatusOK, node)
}

func (s *Server) handleOrgNodeRegister(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return
	}
	var req struct {
		Token            string `json:"token"`
		RegistrationToken string `json:"registration_token"`
		PeerID           string `json:"peer_id"`
		DID              string `json:"did"`
		WalletAddress    string `json:"wallet_address"`
		Chain            string `json:"chain"`
		Name             string `json:"name"`
		Region           string `json:"region"`
		Zone             string `json:"zone"`
		APIBaseURL       string `json:"api_base_url"`
		NodeKey          string `json:"node_key"`
		AccessMode       string `json:"access_mode"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = strings.TrimSpace(req.RegistrationToken)
	}
	if token == "" || req.PeerID == "" || req.DID == "" {
		fail(c, http.StatusBadRequest, "token, peer_id, and did are required")
		return
	}
	orgID := c.Param("id")
	resolvedOrg, tokenID, tokenPeerID, _, err := s.store.LookupNodeRegistrationToken(c, token, store.TokenScopeNodeRegistration)
	if errors.Is(err, store.ErrNotFound) || resolvedOrg != orgID {
		fail(c, http.StatusUnauthorized, "invalid registration token")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "token lookup failed")
		return
	}
	if tokenPeerID != "" && tokenPeerID != strings.TrimSpace(req.PeerID) {
		fail(c, http.StatusUnauthorized, "registration token is bound to another node")
		return
	}
	runtimeID, nodeKey, err := s.store.RegisterOrgNodeFromRuntime(c, orgID, tokenID, store.NodeRegistration{
		PeerID: strings.TrimSpace(req.PeerID), DID: req.DID, Wallet: req.WalletAddress, Chain: req.Chain,
		Name: req.Name, Region: req.Region, Zone: req.Zone, APIBaseURL: req.APIBaseURL,
		NodeKey: req.NodeKey, AccessMode: req.AccessMode,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			fail(c, http.StatusConflict, "node already registered or invalid node_key")
		} else {
			fail(c, http.StatusInternalServerError, "failed to register node")
		}
		return
	}
	ok(c, http.StatusCreated, gin.H{"runtime_node_id": runtimeID, "node_id": req.PeerID, "node_key": nodeKey})
}

func (s *Server) handleListOrgNodeServices(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	services, err := s.store.ListOrgNodeServices(c, c.Param("id"), c.Param("nodeId"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list services")
		return
	}
	ok(c, http.StatusOK, services)
}

func (s *Server) handlePatchOrgNodeService(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return
	}
	var req store.UpdateOrgNodeServiceInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	svc, err := s.store.UpdateOrgNodeService(c, c.Param("id"), c.Param("nodeId"), c.Param("serviceId"), req)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update service")
		return
	}
	ok(c, http.StatusOK, svc)
}

func (s *Server) handleDeleteOrgNodeService(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return
	}
	if err := s.store.DeleteOrgNodeService(c, c.Param("id"), c.Param("nodeId"), c.Param("serviceId")); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "service not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to delete service")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleAttachOrgNodeService(c *gin.Context) {
	role, memberOK := s.orgMember(c)
	if !memberOK {
		return
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return
	}
	var req struct {
		ServiceType     string `json:"service_type"`
		ServiceName     string `json:"service_name"`
		ServiceProvider string `json:"service_provider"`
		Visibility      string `json:"visibility"`
		ConfigRef       string `json:"config_ref"`
		AccessURL       string `json:"access_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ServiceType == "" {
		fail(c, http.StatusBadRequest, "service_type is required")
		return
	}
	orgID := c.Param("id")
	nodeID := c.Param("nodeId")
	if err := s.store.ValidateServiceEntitlement(c, orgID, nodeID, req.ServiceType); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	svc, err := s.store.AttachServiceToNode(c, store.AttachServiceInput{
		OrgID: orgID, NodeID: nodeID, ServiceType: req.ServiceType,
		ServiceName: req.ServiceName, ServiceProvider: req.ServiceProvider,
		Visibility: req.Visibility, ConfigRef: req.ConfigRef, AccessURL: req.AccessURL,
		CreatedBy: userID(c),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to attach service")
		return
	}
	ok(c, http.StatusCreated, svc)
}