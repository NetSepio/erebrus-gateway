package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// nodeOperatorView is the operator's projection of an org node.
type nodeOperatorView struct {
	NodeID        string          `json:"node_id"`
	PeerID        string          `json:"peer_id"`
	DID           string          `json:"did"`
	WalletAddress string          `json:"wallet_address,omitempty"`
	Chain         string          `json:"chain,omitempty"`
	Name          string          `json:"name"`
	Region        string          `json:"region"`
	Zone          string          `json:"zone,omitempty"`
	Status        string          `json:"status"`
	AccessMode    string          `json:"access_mode"`
	DeploymentProfile string      `json:"deployment_profile"` // erebrus(Standard) | shield | sentinel
	MinTier       int             `json:"min_tier"`
	Spec          json.RawMessage `json:"spec"`
	Org           *orgSummary     `json:"org,omitempty"`
	Protocols     []string        `json:"protocols"`
	LoadPct       float64         `json:"load_pct"`
	RxBytes       int64           `json:"rx_bytes"`
	TxBytes       int64           `json:"tx_bytes"`
	Speedtest     json.RawMessage `json:"speedtest"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

func (s *Server) buildNodeOperatorView(c *gin.Context, n *store.Node, callerRole string) nodeOperatorView {
	privileged := store.IsOrgPrivileged(callerRole)
	return nodeOperatorView{
		NodeID: n.PeerID, PeerID: n.PeerID, DID: n.DID, WalletAddress: n.WalletAddress, Chain: n.Chain,
		Name: n.Name, Region: n.Region, Zone: n.Zone, Status: n.Status,
		AccessMode: n.AccessMode, DeploymentProfile: n.DeploymentProfile, MinTier: n.MinTier, Spec: n.Spec,
		Org: s.orgSummaryFor(c, n.OrgID, callerRole, privileged),
		Protocols: n.Protocols, LoadPct: loadPct(n.Load),
		RxBytes: n.RxBytes, TxBytes: n.TxBytes, Speedtest: n.Speedtest,
		LastHeartbeat: n.LastHeartbeat, CreatedAt: n.CreatedAt,
	}
}

// handleOperatorNodes returns nodes for orgs the caller belongs to.
func (s *Server) handleOperatorNodes(c *gin.Context) {
	nodes, err := s.store.OrgNodes(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]nodeOperatorView, 0, len(nodes))
	for _, n := range nodes {
		role, _ := s.store.MemberRole(c, n.OrgID, userID(c))
		out = append(out, s.buildNodeOperatorView(c, n, role))
	}
	ok(c, http.StatusOK, out)
}

// handleOperatorNodeMetrics returns a node's time series; caller must be an org member.
func (s *Server) handleOperatorNodeMetrics(c *gin.Context) {
	nodeID := c.Param("id")
	owned, err := s.store.NodeOperatedBy(c, nodeID, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to check node access")
		return
	}
	if !owned {
		fail(c, http.StatusForbidden, "not your org's node")
		return
	}
	s.writeNodeMetrics(c, nodeID)
}

type patchOperatorNodeReq struct {
	OrgID      *string `json:"org_id"`
	AccessMode *string `json:"access_mode"`
}

// handlePatchOperatorNode updates org attachment or access mode for an org node.
func (s *Server) handlePatchOperatorNode(c *gin.Context) {
	nodeID := c.Param("id")
	node, err := s.store.GetNode(c, nodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load node")
		return
	}
	role, err := s.store.MemberRole(c, node.OrgID, userID(c))
	if errors.Is(err, store.ErrNotFound) || !store.IsOrgPrivileged(role) {
		fail(c, http.StatusForbidden, "org admin role required")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "membership check failed")
		return
	}

	var req patchOperatorNodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.OrgID != nil {
		newOrg := strings.TrimSpace(*req.OrgID)
		if newOrg != "" {
			newRole, err := s.store.MemberRole(c, newOrg, userID(c))
			if errors.Is(err, store.ErrNotFound) || !store.IsOrgPrivileged(newRole) {
				fail(c, http.StatusForbidden, "not admin of target org")
				return
			}
			if err != nil {
				fail(c, http.StatusInternalServerError, "membership check failed")
				return
			}
		}
		if err := s.store.SetNodeOrg(c, nodeID, newOrg); err != nil {
			fail(c, http.StatusInternalServerError, "failed to update org")
			return
		}
	}
	if req.AccessMode != nil {
		mode := strings.TrimSpace(*req.AccessMode)
		if mode != store.NodeAccessPublic && mode != store.NodeAccessPrivate {
			fail(c, http.StatusBadRequest, "access_mode must be public or private")
			return
		}
		if err := s.store.SetNodeAccessMode(c, nodeID, mode); err != nil {
			fail(c, http.StatusInternalServerError, "failed to update access_mode")
			return
		}
	}
	updated, err := s.store.GetNode(c, nodeID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to reload node")
		return
	}
	ok(c, http.StatusOK, s.buildNodeOperatorView(c, updated, role))
}

// handleAdminNodeMetrics returns any node's time series (admin, no ownership check).
func (s *Server) handleAdminNodeMetrics(c *gin.Context) {
	s.writeNodeMetrics(c, c.Param("id"))
}

func (s *Server) writeNodeMetrics(c *gin.Context, nodeID string) {
	rng := parseDuration(c.Query("range"), 24*time.Hour, 90*24*time.Hour)
	step := parseDuration(c.Query("step"), 5*time.Minute, 24*time.Hour)
	points, err := s.store.NodeMetrics(c, nodeID, time.Now().Add(-rng), step)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load metrics")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"node_id": nodeID, "range": rng.String(), "step": step.String(), "points": points,
	})
}

// parseDuration parses a Go duration string, clamped to (0, max], with a default
// for empty/invalid input.
func parseDuration(s string, def, max time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return def
	}
	if d > max {
		return max
	}
	return d
}