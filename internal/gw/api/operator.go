package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// nodeOperatorView is the operator's projection of one of their nodes: full
// operational detail (including private nodes), unlike the public discovery
// projection which hides ownership and excludes private nodes.
type nodeOperatorView struct {
	NodeID        string          `json:"node_id"`
	Name          string          `json:"name"`
	Region        string          `json:"region"`
	Status        string          `json:"status"`
	AccessMode    string          `json:"access_mode"`
	MinTier       int             `json:"min_tier"`
	OrgID         string          `json:"org_id,omitempty"`
	Protocols     []string        `json:"protocols"`
	LoadPct       float64         `json:"load_pct"`
	RxBytes       int64           `json:"rx_bytes"`
	TxBytes       int64           `json:"tx_bytes"`
	Speedtest     json.RawMessage `json:"speedtest"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// handleOperatorNodes returns the caller's nodes (owned directly + via org).
func (s *Server) handleOperatorNodes(c *gin.Context) {
	nodes, err := s.store.OwnedNodes(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list nodes")
		return
	}
	out := make([]nodeOperatorView, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, nodeOperatorView{
			NodeID: n.ID, Name: n.Name, Region: n.Region, Status: n.Status,
			AccessMode: n.AccessMode, MinTier: n.MinTier, OrgID: n.OrgID, Protocols: n.Protocols,
			LoadPct: loadPct(n.Load), RxBytes: n.RxBytes, TxBytes: n.TxBytes,
			Speedtest: n.Speedtest, LastHeartbeat: n.LastHeartbeat, CreatedAt: n.CreatedAt,
		})
	}
	ok(c, http.StatusOK, out)
}

// handleOperatorNodeMetrics returns a node's time series; the caller must operate
// the node (own it or share its org).
func (s *Server) handleOperatorNodeMetrics(c *gin.Context) {
	nodeID := c.Param("id")
	owned, err := s.store.NodeOperatedBy(c, nodeID, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to check node ownership")
		return
	}
	if !owned {
		fail(c, http.StatusForbidden, "not your node")
		return
	}
	s.writeNodeMetrics(c, nodeID)
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
