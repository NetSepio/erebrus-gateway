package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/NetSepio/gateway/internal/gw/token"
	"github.com/NetSepio/gateway/internal/gw/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// nodePublic is the discovery projection: no raw IP, only what a client needs
// to choose and connect to a node.
type nodePublic struct {
	NodeID       string          `json:"node_id"`
	Name         string          `json:"name"`
	DID          string          `json:"did"`
	Region       string          `json:"region"`
	Status       string          `json:"status"`
	Protocols    []string        `json:"protocols"`
	Capabilities json.RawMessage `json:"capabilities"`
	Endpoints    json.RawMessage `json:"endpoints"`
	Speedtest    json.RawMessage `json:"speedtest"`
	LoadPct      float64         `json:"load_pct"`
}

// handleListNodes is the public node directory (Redis-cached, 10s TTL).
func (s *Server) handleListNodes(c *gin.Context) {
	status := c.DefaultQuery("status", "online")
	region := c.Query("region")
	key := "nodes:disco:" + status + ":" + region

	var cached []nodePublic
	if hit, _ := s.cache.GetJSON(c, key, &cached); hit {
		ok(c, http.StatusOK, cached)
		return
	}

	nodes, err := s.store.ListNodes(c, status, region)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]nodePublic, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodePublic{
			NodeID: n.ID, Name: n.Name, DID: n.DID, Region: n.Region, Status: n.Status,
			Protocols: n.Protocols, Capabilities: n.Capabilities, Endpoints: n.Endpoints,
			Speedtest: n.Speedtest, LoadPct: loadPct(n.Load),
		})
	}
	_ = s.cache.SetJSON(c, key, out, 10*time.Second)
	ok(c, http.StatusOK, out)
}

type nodeRegisterReq struct {
	WalletAddress string `json:"wallet_address"`
	Chain         string `json:"chain"`
	// step 2
	FlowID     string `json:"flow_id"`
	Signature  string `json:"signature"`
	PublicKey  string `json:"public_key"`
	PeerID     string `json:"peer_id"`
	DID        string `json:"did"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	APIBaseURL string `json:"api_base_url"`
	APIToken   string `json:"api_token"`
	OrgID      string `json:"org_id"` // optional; operator attaches the node to an org
}

// handleNodeRegister is the two-step node registration (challenge → signed
// response → node PASETO). Step is inferred from the presence of a signature.
func (s *Server) handleNodeRegister(c *gin.Context) {
	var req nodeRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}

	// Step 1: issue a challenge.
	if req.Signature == "" {
		if req.WalletAddress == "" || req.Chain == "" {
			fail(c, http.StatusBadRequest, "wallet_address and chain required")
			return
		}
		flowID := uuid.NewString()
		if err := s.store.CreateFlowID(c, flowID, req.WalletAddress, req.Chain, flowIDTTL); err != nil {
			fail(c, http.StatusInternalServerError, "failed to create challenge")
			return
		}
		ok(c, http.StatusOK, gin.H{"flow_id": flowID, "message": s.challengeMessage(flowID)})
		return
	}

	// Step 2: verify and register.
	if req.PeerID == "" || req.DID == "" {
		fail(c, http.StatusBadRequest, "peer_id and did required")
		return
	}
	flow, err := s.store.GetFlowID(c, req.FlowID)
	if err != nil {
		fail(c, http.StatusUnauthorized, "flow id not found or expired")
		return
	}
	recovered, err := wallet.Verify(flow.Chain, s.challengeMessage(flow.FlowID), req.Signature, req.PublicKey)
	if err != nil || !strings.EqualFold(strings.TrimSpace(recovered), strings.TrimSpace(flow.WalletAddress)) {
		fail(c, http.StatusUnauthorized, "signature does not match wallet")
		return
	}
	_ = s.store.DeleteFlowID(c, req.FlowID)

	// Resolve the operator account from the registering wallet (owner_user_id).
	owner, err := s.store.UpsertUserByWallet(c, flow.WalletAddress, flow.Chain, s.cfg.AdminWalletAddress)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node owner")
		return
	}
	// Optional org attachment — only if the operator belongs to that org.
	orgID := strings.TrimSpace(req.OrgID)
	if orgID != "" {
		if _, err := s.store.MemberRole(c, orgID, owner.ID); err != nil {
			fail(c, http.StatusForbidden, "not a member of the requested org")
			return
		}
	}

	nodeID, err := s.store.RegisterNode(c, store.NodeRegistration{
		PeerID: req.PeerID, DID: req.DID, Wallet: flow.WalletAddress,
		OwnerUserID: owner.ID, OrgID: orgID,
		Name: req.Name, Region: req.Region, APIBaseURL: req.APIBaseURL, APIToken: req.APIToken,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to register node")
		return
	}
	nodeTok, err := s.tokens.IssueNode(nodeID, req.PeerID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to issue node token")
		return
	}
	ok(c, http.StatusOK, gin.H{"node_token": nodeTok, "node_id": nodeID})
}

// handleNodeWS upgrades the node control-plane WebSocket. Node PASETO required
// in the Authorization header of the upgrade request.
func (s *Server) handleNodeWS(c *gin.Context) {
	claims, err := s.tokens.Verify(bearer(c))
	if err != nil || claims.Role != token.RoleNode || claims.NodeID == "" {
		fail(c, http.StatusUnauthorized, "valid node token required")
		return
	}
	s.hub.Serve(c.Writer, c.Request, claims.NodeID, claims.PeerID)
}

// handleAdminNodes lists all nodes with full detail (admin).
func (s *Server) handleAdminNodes(c *gin.Context) {
	nodes, err := s.store.ListNodes(c, c.Query("status"), c.Query("region"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	ok(c, http.StatusOK, gin.H{"online_connected": s.hub.Online(), "nodes": nodes})
}

type nodeCommandReq struct {
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args"`
}

// handleAdminNodeCommand dispatches a control command to a connected node.
func (s *Server) handleAdminNodeCommand(c *gin.Context) {
	var req nodeCommandReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Action == "" {
		fail(c, http.StatusBadRequest, "action required")
		return
	}
	reqID := uuid.NewString()
	if !s.hub.SendCommand(c.Param("id"), req.Action, req.Args, reqID) {
		fail(c, http.StatusConflict, "node not connected")
		return
	}
	ok(c, http.StatusAccepted, gin.H{"request_id": reqID})
}

// loadPct derives a coarse 0-100 load indicator from a node's load JSON.
func loadPct(raw json.RawMessage) float64 {
	if len(raw) == 0 {
		return 0
	}
	var l struct {
		CPUPct float64 `json:"cpu_pct"`
		MemPct float64 `json:"mem_pct"`
	}
	_ = json.Unmarshal(raw, &l)
	if l.MemPct > l.CPUPct {
		return l.MemPct
	}
	return l.CPUPct
}
