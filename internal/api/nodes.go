package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/config"
	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/nodehub"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/NetSepio/gateway/internal/wallet"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// nodePublic is the public discovery projection for pickers and dashboards.
// Excludes raw IP, org_id, enrollment secrets, and full spec blobs.
type nodePublic struct {
	NodeID            string          `json:"node_id"`
	Name              string          `json:"name"`
	DID               string          `json:"did"`
	PeerID            string          `json:"peer_id,omitempty"`
	WalletAddress     string          `json:"wallet_address,omitempty"`
	Chain             string          `json:"chain,omitempty"`
	Region            string          `json:"region"`
	Zone              string          `json:"zone,omitempty"`
	Status            string          `json:"status"`
	AccessMode        string          `json:"access_mode"`
	DeploymentProfile string          `json:"deployment_profile"`
	MinTier           int             `json:"min_tier"`
	Protocols         []string        `json:"protocols"`
	Capabilities      json.RawMessage `json:"capabilities"`
	Endpoints         json.RawMessage `json:"endpoints"`
	IPHash            string          `json:"ip_hash,omitempty"`
	Version           string          `json:"version,omitempty"`
	LoadPct           float64         `json:"load_pct"`
	WGPeersRegistered int             `json:"wg_peers_registered"`
	WGPeersConnected  int             `json:"wg_peers_connected"`
	AcceptingClients  bool            `json:"accepting_clients"`
	RxBytes           int64           `json:"rx_bytes,omitempty"`
	TxBytes           int64           `json:"tx_bytes,omitempty"`
	Speedtest         json.RawMessage `json:"speedtest,omitempty"`
	LastHeartbeat     *time.Time      `json:"last_heartbeat,omitempty"`
	LastPeerHandshake *time.Time      `json:"last_peer_handshake,omitempty"`
	CreatedAt         time.Time       `json:"created_at,omitempty"`
	Org               *orgSummary     `json:"org,omitempty"`
}

func nodePublicFrom(n *store.Node, org *orgSummary, cfg config.PlatformValues) nodePublic {
	load := parseLoad(n.Load)
	return nodePublic{
		NodeID: n.PeerID, Name: n.Name, DID: n.DID, PeerID: n.PeerID, WalletAddress: n.WalletAddress, Chain: n.Chain,
		Region: n.Region, Zone: n.Zone, Status: n.Status, AccessMode: n.AccessMode, DeploymentProfile: n.DeploymentProfile, MinTier: n.MinTier,
		Protocols: n.Protocols, Capabilities: n.Capabilities,
		Endpoints: n.Endpoints, IPHash: n.IPHash, Version: n.Version,
		LoadPct: load.CPUPct, WGPeersRegistered: load.Registered, WGPeersConnected: load.Connected,
		AcceptingClients: acceptingClients(cfg, load.Registered, load.Connected, load.CPUPct),
		RxBytes:          n.RxBytes, TxBytes: n.TxBytes, Speedtest: n.Speedtest,
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
	cfg := s.platform.Snapshot()
	out := make([]nodePublic, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodePublicFrom(n, s.orgSummaryFor(c, n.OrgID, "", false), cfg))
	}
	_ = s.cache.SetJSON(c, key, out, 10*time.Second)
	ok(c, http.StatusOK, out)
}

type nodeRegisterReq struct {
	RegistrationToken string `json:"registration_token"`
	PeerID            string `json:"peer_id"`
	// step 2
	FlowID            string `json:"flow_id"`
	Signature         string `json:"signature"`
	PublicKey         string `json:"public_key"`
	WalletAddress     string `json:"wallet_address"`
	Chain             string `json:"chain"`
	DID               string `json:"did"`
	Name              string `json:"name"`
	Region            string `json:"region"`
	Zone              string `json:"zone"`
	APIBaseURL        string `json:"api_base_url"`
	NodeKey           string `json:"node_key"`
	AccessMode        string `json:"access_mode"` // public | private
	DeploymentProfile string `json:"deployment_profile"`
}

func (s *Server) nodeChallengeMessage(flowID string) string {
	return "Erebrus Node Enrollment Challenge: " + flowID
}

// handleNodeRegister is the two-step node enrollment (challenge → machine-signed
// response → node PASETO). Gated by a scoped org registration token.
func (s *Server) handleNodeRegister(c *gin.Context) {
	var req nodeRegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	token := strings.TrimSpace(req.RegistrationToken)
	if token == "" {
		fail(c, http.StatusBadRequest, "registration_token required")
		return
	}
	resolvedOrgID, regTokenID, tokenPeerID, _, err := s.store.LookupNodeRegistrationToken(c, token, store.TokenScopeNodeRegistration)
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
		if tokenPeerID != "" && tokenPeerID != peerID {
			fail(c, http.StatusUnauthorized, "registration token is bound to another node")
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
	access := req.AccessMode
	if access != store.NodeAccessPrivate {
		access = store.NodeAccessPublic
	}

	env := s.cfg.Environment
	peerID := strings.TrimSpace(req.PeerID)
	if tokenPeerID != "" && tokenPeerID != peerID {
		fail(c, http.StatusUnauthorized, "registration token is bound to another node")
		return
	}
	peerID, nodeKey, err = s.store.RegisterOrgNodeFromRuntime(c, resolvedOrgID, regTokenID, store.NodeRegistration{
		PeerID: peerID, DID: req.DID, Wallet: req.WalletAddress, Chain: nodeChain,
		OrgID: resolvedOrgID, Name: req.Name, Region: req.Region, Zone: req.Zone,
		APIBaseURL: req.APIBaseURL, NodeKey: nodeKey, AccessMode: access,
		DeploymentProfile: req.DeploymentProfile,
	})
	if err != nil {
		metrics.NodeRegistrationsTotal.WithLabelValues("failed", env).Inc()
		if errors.Is(err, store.ErrConflict) {
			fail(c, http.StatusUnauthorized, "node already registered or invalid node_key")
		} else {
			fail(c, http.StatusInternalServerError, "failed to register node")
		}
		return
	}
	nodeTok, err := s.tokens.IssueNode(peerID)
	if err != nil {
		metrics.NodeRegistrationsTotal.WithLabelValues("failed", env).Inc()
		fail(c, http.StatusInternalServerError, "failed to issue node token")
		return
	}
	metrics.NodeRegistrationsTotal.WithLabelValues("success", env).Inc()
	s.cache.DelPrefix(c, "nodes:disco:")
	ok(c, http.StatusOK, gin.H{
		"node_token":         nodeTok,
		"node_id":            peerID,
		"peer_id":            peerID,
		"node_key":           nodeKey,
		"gateway_public_key": s.tokens.PublicKeyHex(),
	})
}

type nodeTokenRefreshReq struct {
	PeerID string `json:"peer_id"`
}

// handleNodeTokenRefresh re-issues a node control-plane PASETO when the prior
// token expired. Authenticated with the node's long-lived node_key bearer.
func (s *Server) handleNodeTokenRefresh(c *gin.Context) {
	var req nodeTokenRefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	peerID := strings.TrimSpace(req.PeerID)
	if peerID == "" {
		fail(c, http.StatusBadRequest, "peer_id required")
		return
	}
	nodeKey := strings.TrimSpace(bearer(c))
	if nodeKey == "" {
		nodeKey = strings.TrimSpace(c.GetHeader("X-Erebrus-Node-Key"))
	}
	if nodeKey == "" {
		fail(c, http.StatusUnauthorized, "node_key required")
		return
	}
	matches, err := s.store.NodeKeyMatches(c, peerID, nodeKey)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "node lookup failed")
		return
	}
	if !matches {
		fail(c, http.StatusUnauthorized, "invalid node_key")
		return
	}
	nodeTok, err := s.tokens.IssueNode(peerID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to issue node token")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"node_token": nodeTok,
		"node_id":    peerID,
		"peer_id":    peerID,
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

// allowedNodeCommandActions restricts the actions an admin may send to a node.
var allowedNodeCommandActions = map[string]bool{
	nodehub.ActionDrain:                    true,
	nodehub.ActionUndrain:                  true,
	nodehub.ActionRotateReality:            true,
	nodehub.ActionResyncPeers:              true,
	nodehub.ActionSyncFirewall:             true,
	nodehub.ActionRestartFirewall:          true,
	nodehub.ActionResetFirewallCredentials: true,
	nodehub.ActionSetFirewallCredentials:   true,
}

// handleAdminNodeCommand dispatches a control command to a connected node.
func (s *Server) handleAdminNodeCommand(c *gin.Context) {
	var req nodeCommandReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Action == "" {
		fail(c, http.StatusBadRequest, "action required")
		return
	}
	if !allowedNodeCommandActions[req.Action] {
		fail(c, http.StatusBadRequest, "invalid action")
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
	s.cache.DelPrefix(c, "nodes:disco:")
	ok(c, http.StatusOK, gin.H{"node_id": c.Param("id"), "min_tier": req.MinTier})
}

// enrichEndpointsForDiscovery injects a wireguard host from the node's IP if the
// node did not advertise one. Used for operator-facing views (not public discovery).
func enrichEndpointsForDiscovery(raw json.RawMessage, ip string) json.RawMessage {
	if strings.TrimSpace(ip) == "" {
		return raw
	}
	m := make(map[string]any)
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			m = make(map[string]any)
		}
	}
	wg, _ := m["wireguard"].(map[string]any)
	if wg == nil {
		wg = make(map[string]any)
		m["wireguard"] = wg
	}
	if _, ok := wg["host"]; !ok {
		wg["host"] = ip
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

type parsedLoad struct {
	CPUPct     float64
	Registered int
	Connected  int
}

func parseLoad(load json.RawMessage) parsedLoad {
	if len(load) == 0 {
		return parsedLoad{}
	}
	var l struct {
		CPUPct            float64 `json:"cpu_pct"`
		WGPeersRegistered int     `json:"wg_peers_registered"`
		WGPeersConnected  int     `json:"wg_peers_connected"`
		WGPeers           int     `json:"wg_peers"` // legacy field from old nodes
	}
	if err := json.Unmarshal(load, &l); err != nil {
		return parsedLoad{}
	}
	registered := l.WGPeersRegistered
	if registered == 0 && l.WGPeers > 0 {
		registered = l.WGPeers
	}
	// Missing wg_peers_connected defaults to 0 per business decision: the node
	// reports the actual count, and legacy/empty values represent no connected peers.
	connected := l.WGPeersConnected
	return parsedLoad{CPUPct: l.CPUPct, Registered: registered, Connected: connected}
}

// loadPct returns the node's CPU load percentage from its heartbeat payload.
func loadPct(load json.RawMessage) float64 {
	return parseLoad(load).CPUPct
}

// acceptingClients decides whether a node should accept new VPN clients based
// on the latest heartbeat. Reconnects are handled separately in the provision
// path and are not gated by this predicate.
func acceptingClients(cfg config.PlatformValues, registered, connected int, cpuPct float64) bool {
	if cpuPct > cfg.NodeCPUMax {
		return false
	}
	if registered > 0 && float64(connected)/float64(registered) >= cfg.NodePeerRatioMax {
		return false
	}
	if cpuPct > cfg.NodeCPUSoft && connected >= cfg.NodePeerConnectedSoft {
		return false
	}
	return true
}
