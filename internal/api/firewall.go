package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/nodehub"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (s *Server) requireNodeOperator(c *gin.Context) (string, bool) {
	role, ok := s.orgMember(c)
	if !ok {
		return "", false
	}
	if role != store.OrgRoleOwner && role != store.OrgRoleAdmin && role != store.OrgRoleNodeOperator {
		fail(c, http.StatusForbidden, "insufficient role")
		return "", false
	}
	return role, true
}

func (s *Server) handleGetFirewall(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	fw, err := s.store.GetFirewallService(c, c.Param("id"), c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	ok(c, http.StatusOK, fw)
}

func (s *Server) handleGetFirewallStatus(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	fw, err := s.store.GetFirewallService(c, c.Param("id"), c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall status")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"service_status": fw.Service.ServiceStatus,
		"service_kind":   fw.ServiceKind,
		"access_url":     fw.Service.AccessURL,
		"config_ref":     fw.Service.ConfigRef,
	})
}

func (s *Server) handleFirewallRestart(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	orgID, nodeID := c.Param("id"), c.Param("nodeId")
	if _, err := s.store.GetFirewallService(c, orgID, nodeID); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	sent := s.hub.SendCommand(nodeID, nodehub.ActionRestartFirewall, nil, uuid.NewString())
	svc, err := s.store.UpdateFirewallServiceStatus(c, orgID, nodeID, store.ServiceStatusActive, nil, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to restart firewall")
		return
	}
	ok(c, http.StatusAccepted, gin.H{"service": svc, "action": "restart", "node_notified": sent})
}

func (s *Server) handleFirewallSync(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	orgID, nodeID := c.Param("id"), c.Param("nodeId")
	fw, err := s.store.GetFirewallService(c, orgID, nodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	rules, err := s.store.ListFirewallRules(c, orgID, nodeID, fw.Service.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall rules")
		return
	}
	payload := store.BuildFirewallSyncPayload(orgID, nodeID, fw, rules)
	args, err := store.MarshalFirewallSyncPayload(payload)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to build sync payload")
		return
	}
	sent := s.hub.SendCommand(nodeID, nodehub.ActionSyncFirewall, args, uuid.NewString())
	svc, err := s.store.UpdateFirewallServiceStatus(c, orgID, nodeID, store.ServiceStatusActive, nil, nil)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to sync firewall")
		return
	}
	ok(c, http.StatusAccepted, gin.H{"service": svc, "action": "sync", "node_notified": sent, "rules": len(payload.Rules)})
}

func (s *Server) handleFirewallResetCredentials(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	orgID, nodeID := c.Param("id"), c.Param("nodeId")
	if _, err := s.store.GetFirewallService(c, orgID, nodeID); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	empty := ""
	sent := s.hub.SendCommand(nodeID, nodehub.ActionResetFirewallCredentials, nil, uuid.NewString())
	svc, err := s.store.UpdateFirewallServiceStatus(c, orgID, nodeID, store.ServiceStatusActive, &empty, &empty)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to reset credentials")
		return
	}
	ok(c, http.StatusAccepted, gin.H{"service": svc, "action": "reset_credentials", "node_notified": sent})
}

func (s *Server) handleListFirewallRules(c *gin.Context) {
	if _, ok := s.orgMember(c); !ok {
		return
	}
	fw, err := s.store.GetFirewallService(c, c.Param("id"), c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	if fw.ServiceKind != "sentinel" {
		fail(c, http.StatusBadRequest, "rule management requires Sentinel firewall")
		return
	}
	rules, err := s.store.ListFirewallRules(c, c.Param("id"), c.Param("nodeId"), fw.Service.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list rules")
		return
	}
	ok(c, http.StatusOK, rules)
}

func (s *Server) handleCreateFirewallRule(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	var req struct {
		RuleType string `json:"rule_type"`
		Target   string `json:"target"`
		Action   string `json:"action"`
		Scope    string `json:"scope"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RuleType == "" || req.Target == "" {
		fail(c, http.StatusBadRequest, "rule_type and target are required")
		return
	}
	fw, err := s.store.GetFirewallService(c, c.Param("id"), c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "firewall service not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load firewall")
		return
	}
	if fw.ServiceKind != "sentinel" {
		fail(c, http.StatusBadRequest, "rule management requires Sentinel firewall")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	action := req.Action
	if action == "" {
		action = "apply"
	}
	scope := req.Scope
	if scope == "" {
		scope = "node"
	}
	rule, err := s.store.CreateFirewallRule(c, store.CreateFirewallRuleInput{
		OrgID: c.Param("id"), NodeID: c.Param("nodeId"), FirewallServiceID: fw.Service.ID,
		RuleType: req.RuleType, Target: req.Target, Action: action, Scope: scope,
		Enabled: enabled, CreatedBy: userID(c),
	})
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	ok(c, http.StatusCreated, rule)
}

func (s *Server) handlePatchFirewallRule(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	var req store.UpdateFirewallRuleInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	rule, err := s.store.UpdateFirewallRule(c, c.Param("id"), c.Param("ruleId"), req)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "rule not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update rule")
		return
	}
	ok(c, http.StatusOK, rule)
}

func (s *Server) handleDeleteFirewallRule(c *gin.Context) {
	if _, ok := s.requireNodeOperator(c); !ok {
		return
	}
	if err := s.store.DeleteFirewallRule(c, c.Param("id"), c.Param("ruleId")); errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "rule not found")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, "failed to delete rule")
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleNodeHeartbeat(c *gin.Context) {
	claims, err := s.tokens.Verify(bearer(c))
	peerID := nodeTokenPeerID(claims)
	if err != nil || claims.Role != token.RoleNode || peerID == "" {
		fail(c, http.StatusUnauthorized, "valid node token required")
		return
	}
	if param := c.Param("nodeId"); param != "" && param != peerID {
		fail(c, http.StatusForbidden, "node id mismatch")
		return
	}
	var req struct {
		Status    string          `json:"status"`
		Load      json.RawMessage `json:"load"`
		Speedtest json.RawMessage `json:"speedtest"`
		RxBytes   int64           `json:"rx_bytes"`
		TxBytes   int64           `json:"tx_bytes"`
		Version   string          `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	status := req.Status
	if status == "" {
		status = "online"
	}
	if err := s.store.ApplyNodeHeartbeat(c, peerID, status, req.Load, req.Speedtest, req.RxBytes, req.TxBytes, req.Version); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "node not found")
			return
		}
		fail(c, http.StatusInternalServerError, "heartbeat failed")
		return
	}
	c.Status(http.StatusNoContent)
}