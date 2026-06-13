package api

import (
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/gw/nodeclient"
	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/NetSepio/gateway/internal/gw/token"
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

	// Entitlement: must have an active subscription/trial.
	sub, err := s.store.ActiveSubscription(c, uid)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusPaymentRequired, "no active subscription — start a trial or subscribe")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "entitlement check failed")
		return
	}
	// Plan client limit.
	if plan, err := s.store.GetPlan(c, sub.PlanID); err == nil {
		if n, _ := s.store.CountActiveClientsByUser(c, uid); n >= plan.MaxClients {
			fail(c, http.StatusConflict, "client limit reached for your plan")
			return
		}
	}

	// Resolve node API endpoint.
	baseURL, nodeToken, status, err := s.store.NodeAPI(c, req.NodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	if status == "draining" {
		fail(c, http.StatusConflict, "node is draining")
		return
	}
	if baseURL == "" {
		fail(c, http.StatusBadGateway, "node has no reachable API endpoint")
		return
	}

	// Commit a pending client row; its id is the peer id used on the node.
	clientID, err := s.store.CreateClient(c, uid, "", req.NodeID, req.Name, req.WGPublicKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to create client")
		return
	}

	bundle, err := s.nodes.UpsertPeer(c, baseURL, nodeToken, clientID, nodeclient.PeerRequest{
		Name: req.Name, WGPublicKey: req.WGPublicKey, WGPresharedKey: req.WGPresharedKey,
	})
	if err != nil {
		_ = s.store.DeleteClient(c, clientID) // roll back the pending row
		fail(c, http.StatusBadGateway, "node unreachable — no client created")
		return
	}
	if err := s.store.SetClientActive(c, clientID, bundle.WGAddress); err != nil {
		fail(c, http.StatusInternalServerError, "failed to activate client")
		return
	}
	c.Data(http.StatusCreated, "application/json", bundle.Raw)
}

// handleDeleteClient removes a client from the gateway and the node.
func (s *Server) handleDeleteClient(c *gin.Context) {
	cl, err := s.ownedClient(c)
	if err != nil {
		return // response already written
	}
	if baseURL, nodeToken, _, err := s.store.NodeAPI(c, cl.NodeID); err == nil && baseURL != "" {
		_ = s.nodes.DeletePeer(c, baseURL, nodeToken, cl.ID) // best-effort
	}
	if err := s.store.DeleteClient(c, cl.ID); err != nil {
		fail(c, http.StatusInternalServerError, "failed to delete client")
		return
	}
	c.Status(http.StatusNoContent)
}

// handleClientConfig re-fetches a client's credential bundle from its node.
func (s *Server) handleClientConfig(c *gin.Context) {
	cl, err := s.ownedClient(c)
	if err != nil {
		return
	}
	baseURL, nodeToken, _, err := s.store.NodeAPI(c, cl.NodeID)
	if err != nil || baseURL == "" {
		fail(c, http.StatusBadGateway, "node unreachable")
		return
	}
	raw, err := s.nodes.Credentials(c, baseURL, nodeToken, cl.ID)
	if err != nil {
		fail(c, http.StatusNotFound, "credentials not available")
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
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
