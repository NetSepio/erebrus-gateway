package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/middleware"
	"github.com/NetSepio/gateway/internal/nodeclient"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/NetSepio/gateway/internal/token"
	"github.com/gin-gonic/gin"
)

// handleListClients returns the caller's VPN clients.
func (s *Server) handleListClients(c *gin.Context) {
	clients, err := s.store.ListClientsByUser(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list clients")
		return
	}
	ok(c, http.StatusOK, clients)
}

type provisionReq struct {
	Name           string `json:"name"`
	NodeID         string `json:"node_id"`
	WGPublicKey    string `json:"wg_public_key"`
	WGPresharedKey string `json:"wg_preshared_key"`
	IdempotencyKey string `json:"idempotency_key"`
}

// handleProvisionClient provisions a VPN client on a node. Entitlement-gated,
// then commits a pending row, calls the node (idempotent, retried), and
// activates on success.
func (s *Server) handleProvisionClient(c *gin.Context) {
	var req provisionReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" || req.NodeID == "" || req.WGPublicKey == "" {
		fail(c, http.StatusBadRequest, "name, node_id and wg_public_key are required")
		return
	}
	uid := userID(c)
	isAdmin := c.GetString(ctxRole) == token.RoleAdmin

	// Resolve the node first; entitlement depends on its access mode.
	node, err := s.store.GetNode(c, req.NodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}

	// Entitlement (organization-only — no personal trial/subscription/NFT):
	//   - admin: always.
	//   - private node: any member of the node's org (membership covers the org's
	//     own private nodes — no seat needed).
	//   - public node: any active organization membership; the highest
	//     organization/seat tier controls product limits.
	if !isAdmin {
		if node.AccessMode == store.NodeAccessPrivate {
			member, err := s.store.UserCanProvisionNode(c, node.PeerID, uid)
			if err != nil {
				fail(c, http.StatusInternalServerError, "node access check failed")
				return
			}
			if !member {
				fail(c, http.StatusForbidden, "private node — org membership required")
				return
			}
		} else {
			orgPlan, perr := s.store.UserOrgVPNPlan(c, uid)
			if perr != nil {
				fail(c, http.StatusInternalServerError, "entitlement check failed")
				return
			}
			if orgPlan == "" {
				fail(c, http.StatusPaymentRequired, "active organization membership is required to use public nodes")
				return
			}
		}
	}

	// Tier-gated premium pool: a node may require a minimum tier to connect.
	if !isAdmin && node.MinTier > 0 {
		if _, _, tier, _ := s.store.UserXP(c, uid); tier < node.MinTier {
			fail(c, http.StatusForbidden, "node requires a higher tier")
			return
		}
	}

	// Reconnect idempotency: same device (WG pubkey) on the same node reuses the
	// existing gateway client row instead of minting a new peer id on the node.
	// (Per-subscription client caps are retired with personal subscriptions;
	// organization seats govern access.)
	s.doProvision(c, uid, "", node.PeerID, req.Name, req.WGPublicKey, req.WGPresharedKey)
}

// doProvision performs the node-proxied provisioning steps shared by the user
// and org (API-key) paths: resolve the node, commit a pending client, call the
// node (idempotent, retried), then activate. Writes the response.
func (s *Server) doProvision(c *gin.Context, uid, org, nodeID, name, wgPub, wgPSK string) {
	client := middleware.DetectClient(c)
	region := vpnRegion(c, s, nodeID)
	env := s.cfg.Environment
	recordVPN := func(status string) {
		metrics.VPNConfigsGeneratedTotal.WithLabelValues(client, region, status, env).Inc()
	}

	node, err := s.store.GetNode(c, nodeID)
	if errors.Is(err, store.ErrNotFound) {
		recordVPN("failed")
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	internalID := node.ID
	peerID := node.PeerID

	baseURL, nodeKey, status, err := s.store.NodeAPI(c, peerID)
	if errors.Is(err, store.ErrNotFound) {
		recordVPN("failed")
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	if status == "draining" {
		recordVPN("failed")
		fail(c, http.StatusConflict, "node is draining")
		return
	}
	if baseURL == "" {
		recordVPN("failed")
		fail(c, http.StatusBadGateway, "node has no reachable API endpoint")
		return
	}

	// Reuse an existing client row for the same device, or commit a new pending row.
	// Its id is the peer id used on the node.
	var clientID string
	if existing, err := s.store.FindClientByUserNodeWGKey(c, uid, internalID, wgPub); err == nil && existing != nil {
		clientID = existing.ID
		_ = s.store.DeletePendingClientsByUserNodeWGKey(c, uid, internalID, wgPub, clientID)
	} else {
		// Capacity gate: reject new clients on a full node. Reconnects (existing
		// row above) are always allowed.
		cfg := s.platform.Snapshot()
		load := parseLoad(node.Load)
		if !acceptingClients(cfg, load.Registered, load.Connected, load.CPUPct) {
			recordVPN("failed")
			fail(c, http.StatusServiceUnavailable, "node is at capacity — try another node")
			return
		}
		var err error
		clientID, err = s.store.CreateClient(c, uid, org, internalID, name, wgPub)
		if err != nil {
			recordVPN("failed")
			fail(c, http.StatusInternalServerError, "failed to create client")
			return
		}
	}
	gwTok, err := s.tokens.IssueGatewayCall(peerID, "peer_upsert")
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusInternalServerError, "failed to sign gateway call")
		return
	}
	peerReq := nodeclient.PeerRequest{Name: name, WGPublicKey: wgPub, WGPresharedKey: wgPSK}
	bundle, err := s.upsertPeerWithFallback(c, peerID, baseURL, gwTok, nodeKey, clientID, peerReq)
	if err != nil {
		_ = s.store.DeleteClient(c, clientID) // roll back the pending row
		recordVPN("failed")
		fail(c, http.StatusBadGateway, "node unreachable — no client created")
		return
	}
	if err := s.store.SetClientActive(c, clientID, bundle.WGAddress); err != nil {
		recordVPN("failed")
		fail(c, http.StatusInternalServerError, "failed to activate client")
		return
	}
	recordVPN("success")
	c.Data(http.StatusCreated, "application/json", bundle.Raw)
}

// handleDeleteClient removes a client from the gateway and the node.
func (s *Server) handleDeleteClient(c *gin.Context) {
	cl, err := s.ownedClient(c)
	if err != nil {
		return // response already written
	}
	if node, err := s.store.GetNode(c, cl.NodeID); err == nil {
		if baseURL, nodeKey, _, err := s.store.NodeAPI(c, cl.NodeID); err == nil && baseURL != "" {
			if gwTok, err := s.tokens.IssueGatewayCall(node.PeerID, "peer_delete"); err == nil {
				_ = s.nodes.DeletePeer(c, baseURL, gwTok, nodeKey, cl.ID) // best-effort
			}
		}
	}
	if err := s.store.DeleteClient(c, cl.ID); err != nil {
		fail(c, http.StatusInternalServerError, "failed to delete client")
		return
	}
	c.Status(http.StatusNoContent)
}

// handleClientConfig re-fetches a client's credential bundle from its node.
func (s *Server) handleClientConfig(c *gin.Context) {
	client := middleware.DetectClient(c)
	env := s.cfg.Environment
	cl, err := s.ownedClient(c)
	if err != nil {
		return
	}
	region := vpnRegion(c, s, cl.NodeID)
	recordVPN := func(status string) {
		metrics.VPNConfigsGeneratedTotal.WithLabelValues(client, region, status, env).Inc()
	}

	node, err := s.store.GetNode(c, cl.NodeID)
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusBadGateway, "node unreachable")
		return
	}
	baseURL, nodeKey, _, err := s.store.NodeAPI(c, cl.NodeID)
	if err != nil || baseURL == "" {
		recordVPN("failed")
		fail(c, http.StatusBadGateway, "node unreachable")
		return
	}
	gwTok, err := s.tokens.IssueGatewayCall(node.PeerID, "peer_credentials")
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusInternalServerError, "failed to sign gateway call")
		return
	}
	raw, err := s.nodes.Credentials(c, baseURL, gwTok, nodeKey, cl.ID)
	if err != nil {
		recordVPN("failed")
		fail(c, http.StatusNotFound, "credentials not available")
		return
	}
	recordVPN("success")
	c.Data(http.StatusOK, "application/json", raw)
}

func vpnRegion(c *gin.Context, s *Server, nodeID string) string {
	if node, err := s.store.GetNode(c, nodeID); err == nil {
		return metrics.NormalizeRegion(node.Region)
	}
	return "unknown"
}

// upsertPeerWithFallback tries the stored api_base_url first, then fallbacks
// derived from the node's reported IP (covers missing/wrong registration URLs).
func (s *Server) upsertPeerWithFallback(
	c *gin.Context,
	nodeID, baseURL, gatewayToken, nodeKey, clientID string,
	req nodeclient.PeerRequest,
) (*nodeclient.Bundle, error) {
	var lastErr error
	for _, u := range nodeAPICandidates(c, s, nodeID, baseURL) {
		bundle, err := s.nodes.UpsertPeer(c, u, gatewayToken, nodeKey, clientID, req)
		if err == nil {
			return bundle, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func nodeAPICandidates(c *gin.Context, s *Server, nodeID, baseURL string) []string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		return nil
	}
	return []string{u}
}

// ownedClient loads the path :id client and checks the caller owns it (or is
// admin). On failure it writes the error response and returns a non-nil error.
func (s *Server) ownedClient(c *gin.Context) (*store.Client, error) {
	cl, err := s.store.GetClient(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "client not found")
		return nil, err
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load client")
		return nil, err
	}
	if cl.UserID != userID(c) && c.GetString(ctxRole) != token.RoleAdmin {
		fail(c, http.StatusForbidden, "not your client")
		return nil, errors.New("forbidden")
	}
	return cl, nil
}
