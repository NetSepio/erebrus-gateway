package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/secrets"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/NetSepio/gateway/internal/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// nodePublic is the public discovery projection for pickers and dashboards.
// Excludes raw IP, org_id, enrollment secrets, and full spec blobs.
type nodePublic struct {
	NodeID        string          `json:"node_id"`
	Name          string          `json:"name"`
	DID           string          `json:"did"`
	PeerID        string          `json:"peer_id,omitempty"`
	WalletAddress string          `json:"wallet_address,omitempty"`
	Chain         string          `json:"chain,omitempty"`
	Region        string          `json:"region"`
	Zone          string          `json:"zone,omitempty"`
	Status        string          `json:"status"`
	AccessMode    string          `json:"access_mode"`
	MinTier       int             `json:"min_tier"`
	Protocols     []string        `json:"protocols"`
	Capabilities  json.RawMessage `json:"capabilities"`
	Endpoints     json.RawMessage `json:"endpoints"`
	Speedtest     json.RawMessage `json:"speedtest"`
	LoadPct       float64         `json:"load_pct"`
	IPHash        string          `json:"ip_hash,omitempty"`
	Version       string          `json:"version,omitempty"`
	RxBytes       int64           `json:"rx_bytes,omitempty"`
	TxBytes       int64           `json:"tx_bytes,omitempty"`
	LastHeartbeat     *time.Time  `json:"last_heartbeat,omitempty"`
	LastPeerHandshake *time.Time  `json:"last_peer_handshake,omitempty"`
	CreatedAt         time.Time   `json:"created_at,omitempty"`
	Org               *orgSummary `json:"org,omitempty"`
}

func nodePublicFrom(n *store.Node, org *orgSummary) nodePublic {
	return nodePublic{
		NodeID: n.PeerID, Name: n.Name, DID: n.DID, PeerID: n.PeerID, WalletAddress: n.WalletAddress, Chain: n.Chain,
		Region: n.Region, Zone: n.Zone, Status: n.Status, AccessMode: n.AccessMode, MinTier: n.MinTier,
		Protocols: n.Protocols, Capabilities: n.Capabilities,
		Endpoints: enrichEndpointsForDiscovery(n.Endpoints, n.IP), Speedtest: n.Speedtest,
		LoadPct: loadPct(n.Load), IPHash: n.IPHash, Version: n.Version,
		RxBytes: n.RxBytes, TxBytes: n.TxBytes,
		LastHeartbeat: n.LastHeartbeat, LastPeerHandshake: n.LastPeerHandshake, CreatedAt: n.CreatedAt,
		Org: org,
	}
}

// handleListNodes is the public node directory (Redis-cached, 10s TTL).
func (s *Server) handleListNodes(c *gin.Context) {
	status := c.DefaultQuery("status", "online")
	region := c.Query("region")
	zone := c.Query("zone")
	tier := s.callerTier(c)
	key := "nodes:disco:" + status + ":" + region + ":" + zone + ":t" + strconv.Itoa(tier)

	var cached []nodePublic
	if hit, _ := s.cache.GetJSON(c, key, &cached); hit {
		ok(c, http.StatusOK, cached)
		return
	}

	nodes, err := s.store.ListDiscoverableNodes(c, status, region, zone, tier)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]nodePublic, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodePublicFrom(n, s.orgSummaryFor(c, n.OrgID, "", false)))
	}
	_ = s.cache.SetJSON(c, key, out, 10*time.Second)
	ok(c, http.StatusOK, out)
}

type nodeRegisterReq struct {
	EnrollmentSecret  string `json:"enrollment_secret"`
	RegistrationToken string `json:"registration_token"`
	PeerID           string `json:"peer_id"`
	// step 2
	FlowID        string `json:"flow_id"`
	Signature     string `json:"signature"`
	PublicKey     string `json:"public_key"`
	WalletAddress string `json:"wallet_address"`
	Chain         string `json:"chain"`
	DID           string `json:"did"`
	Name          string `json:"name"`
	Region        string `json:"region"`
	Zone          string `json:"zone"`
	APIBaseURL    string `json:"api_base_url"`
	NodeKey       string `json:"node_key"`
	AccessMode    string `json:"access_mode"` // public | private
}

func (s *Server) nodeChallengeMessage(flowID string) string {
	return "Erebrus Node Enrollment Challenge: " + flowID
}

// handleNodeRegister is the two-step node enrollment (challenge → machine-signed
// response → node PASETO). Gated by org enrollment_secret, not human wallet auth.
func (s *Server) handleNodeRegister(c *gin.Context) {
	var req nodeRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	token := strings.TrimSpace(req.RegistrationToken)
	if token == "" {
		token = strings.TrimSpace(req.EnrollmentSecret) // legacy alias
	}
	if token == "" {
		fail(c, http.StatusBadRequest, "registration_token required")
		return
	}
	resolvedOrgID, regTokenID, _, err := s.store.LookupNodeRegistrationToken(c, token, store.TokenScopeNodeRegistration)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusUnauthorized, "invalid registration token")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "token lookup failed")
		return
	}

	// Step 1: issue a machine challenge.
	if req.Signature == "" {
		peerID := strings.TrimSpace(req.PeerID)
		if peerID == "" {
			fail(c, http.StatusBadRequest, "peer_id required")
			return
		}
		flowID := uuid.NewString()
		// flow.Chain stores org_id; flow.WalletAddress stores peer_id for step-2 binding.
		if err := s.store.CreateFlowID(c, flowID, peerID, resolvedOrgID, flowIDTTL); err != nil {
			fail(c, http.StatusInternalServerError, "failed to create challenge")
			return
		}
		ok(c, http.StatusOK, gin.H{
			"flow_id":            flowID,
			"message":            s.nodeChallengeMessage(flowID),
			"gateway_public_key": s.tokens.PublicKeyHex(),
		})
		return
	}

	// Step 2: verify machine signature and register.
	if req.PeerID == "" || req.DID == "" || req.WalletAddress == "" || req.Chain == "" {
		fail(c, http.StatusBadRequest, "peer_id, did, wallet_address and chain required")
		return
	}
	flow, err := s.store.GetFlowID(c, req.FlowID)
	if err != nil {
		fail(c, http.StatusUnauthorized, "flow id not found or expired")
		return
	}
	if flow.Chain != resolvedOrgID {
		fail(c, http.StatusUnauthorized, "enrollment flow org mismatch")
		return
	}
	if !strings.EqualFold(strings.TrimSpace(flow.WalletAddress), strings.TrimSpace(req.PeerID)) {
		fail(c, http.StatusUnauthorized, "peer_id does not match challenge")
		return
	}
	nodeChain, verifyChain, err := wallet.ParseNodeChain(req.Chain)
	if err != nil {
		fail(c, http.StatusBadRequest, "unsupported chain (expected SOLANA or ETHEREUM)")
		return
	}
	recovered, err := wallet.Verify(verifyChain, s.nodeChallengeMessage(flow.FlowID), req.Signature, req.PublicKey)
	if err != nil || !strings.EqualFold(strings.TrimSpace(recovered), strings.TrimSpace(req.WalletAddress)) {
		fail(c, http.StatusUnauthorized, "signature does not match node wallet")
		return
	}
	_ = s.store.DeleteFlowID(c, req.FlowID)

	nodeKey := strings.TrimSpace(req.NodeKey)
	if nodeKey == "" {
		nodeKey, err = secrets.NewNodeKey()
		if err != nil {
			fail(c, http.StatusInternalServerError, "failed to mint node key")
			return
		}
	}
	access := req.AccessMode
	if access != store.NodeAccessPrivate {
		access = store.NodeAccessPublic
	}

	env := s.cfg.Environment
	peerID := strings.TrimSpace(req.PeerID)
	if _, err := s.store.RegisterOrgNodeFromRuntime(c, resolvedOrgID, regTokenID, store.NodeRegistration{
		PeerID: peerID, DID: req.DID, Wallet: req.WalletAddress, Chain: nodeChain,
		OrgID: resolvedOrgID, Name: req.Name, Region: req.Region, Zone: req.Zone,
		APIBaseURL: req.APIBaseURL, NodeKey: nodeKey, AccessMode: access,
	}); err != nil {
		metrics.NodeRegistrationsTotal.WithLabelValues("failed", env).Inc()
		fail(c, http.StatusInternalServerError, "failed to register node")
		return
	}
	nodeTok, err := s.tokens.IssueNode(peerID)
	if err != nil {
		metrics.NodeRegistrationsTotal.WithLabelValues("failed", env).Inc()
		fail(c, http.StatusInternalServerError, "failed to issue node token")
		return
	}
	metrics.NodeRegistrationsTotal.WithLabelValues("success", env).Inc()
	ok(c, http.StatusOK, gin.H{
		"node_token":         nodeTok,
		"node_id":            peerID,
		"peer_id":            peerID,
		"node_key":           nodeKey,
		"gateway_public_key": s.tokens.PublicKeyHex(),
	})
}

// handleNodeWS upgrades the node control-plane WebSocket. Node PASETO required
// in the Authorization header of the upgrade request.
func (s *Server) handleNodeWS(c *gin.Context) {
	claims, err := s.tokens.Verify(bearer(c))
	peerID := nodeTokenPeerID(claims)
	if err != nil || claims.Role != token.RoleNode || peerID == "" {
		fail(c, http.StatusUnauthorized, "valid node token required")
		return
	}
	s.hub.Serve(c.Writer, c.Request, peerID)
}

// nodeTokenPeerID returns the canonical peer_id from node or gateway-call claims.
func nodeTokenPeerID(claims *token.Claims) string {
	if claims == nil {
		return ""
	}
	if claims.PeerID != "" {
		return claims.PeerID
	}
	return claims.NodeID
}

// handleAdminNodes lists all nodes with full detail (admin).
func (s *Server) handleAdminNodes(c *gin.Context) {
	nodes, err := s.store.ListNodes(c, c.Query("status"), c.Query("region"), c.Query("zone"))
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
	peerID, err := s.store.ResolvePeerID(c, c.Param("id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "node not found")
			return
		}
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	if !s.hub.SendCommand(peerID, req.Action, req.Args, reqID) {
		fail(c, http.StatusConflict, "node not connected")
		return
	}
	ok(c, http.StatusAccepted, gin.H{"request_id": reqID})
}

type setMinTierReq struct {
	MinTier int `json:"min_tier"`
}

// handleAdminSetNodeMinTier sets a node's premium-pool tier gate (admin only).
func (s *Server) handleAdminSetNodeMinTier(c *gin.Context) {
	var req setMinTierReq
	if err := c.ShouldBindJSON(&req); err != nil || req.MinTier < 0 {
		fail(c, http.StatusBadRequest, "min_tier must be >= 0")
		return
	}
	if err := s.store.SetNodeMinTier(c, c.Param("id"), req.MinTier); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "node not found")
			return
		}
		fail(c, http.StatusInternalServerError, "failed to set min_tier")
		return
	}
	ok(c, http.StatusOK, gin.H{"node_id": c.Param("id"), "min_tier": req.MinTier})
}

// enrichEndpointsForDiscovery adds the dial host under wireguard.host so clients
// can measure phone→node RTT without exposing a separate top-level IP field.
func enrichEndpointsForDiscovery(raw json.RawMessage, ip string) json.RawMessage {
	host := strings.TrimSpace(ip)
	if host == "" {
		return raw
	}
	if len(raw) == 0 {
		out, err := json.Marshal(map[string]any{"wireguard": map[string]any{"host": host}})
		if err != nil {
			return raw
		}
		return out
	}
	var eps map[string]any
	if err := json.Unmarshal(raw, &eps); err != nil {
		return raw
	}
	wg, ok := eps["wireguard"].(map[string]any)
	if !ok {
		wg = map[string]any{}
	}
	if existing, _ := wg["host"].(string); strings.TrimSpace(existing) == "" {
		wg["host"] = host
		eps["wireguard"] = wg
	}
	out, err := json.Marshal(eps)
	if err != nil {
		return raw
	}
	return out
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
