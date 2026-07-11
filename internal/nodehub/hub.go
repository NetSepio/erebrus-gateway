package nodehub

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NetSepio/gateway/internal/cache"
	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/gorilla/websocket"
)

const (
	heartbeatIntervalSec = 30
	writeWait            = 10 * time.Second
	pongWait             = 95 * time.Second // ~3 missed heartbeats
	pingPeriod           = 27 * time.Second
	sendQueue            = 16
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Use the default same-origin policy. Auth is via PASETO, but there is no
	// benefit to allowing arbitrary origins to open the WebSocket.
}

// Hub tracks live node connections and persists their control-plane reports.
type Hub struct {
	store       *store.Store
	cache       *cache.Cache
	log         *slog.Logger
	environment string

	mu           sync.RWMutex
	conns        map[string]*conn // keyed by nodeID
	regionCounts map[string]int
}

// New constructs a Hub.
func New(st *store.Store, c *cache.Cache, log *slog.Logger, environment string) *Hub {
	if log == nil {
		log = slog.Default()
	}
	if environment == "" {
		environment = "dev"
	}
	return &Hub{
		store: st, cache: c, log: log, environment: environment,
		conns: map[string]*conn{}, regionCounts: map[string]int{},
	}
}

// Online reports how many nodes are currently connected.
func (h *Hub) Online() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}

// SendCommand queues a command to a connected node. Returns false if the node
// is not currently connected.
func (h *Hub) SendCommand(peerID, action string, args json.RawMessage, requestID string) bool {
	h.mu.RLock()
	c := h.conns[peerID]
	h.mu.RUnlock()
	if c == nil {
		return false
	}
	frame, err := wrap(TypeCommand, Command{Action: action, RequestID: requestID, Args: args})
	if err != nil {
		return false
	}
	select {
	case c.send <- frame:
		return true
	default:
		return false // backpressure: node is slow, drop
	}
}

// Serve upgrades an authenticated request to a node WebSocket and runs the
// read/write pumps until the connection closes. peerID is the canonical node id.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, peerID string) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("node ws upgrade failed", "peer_id", peerID, "err", err)
		return
	}
	c := &conn{peerID: peerID, ws: ws, send: make(chan []byte, sendQueue), hub: h}

	h.mu.Lock()
	if old := h.conns[peerID]; old != nil {
		h.adjustRegionLocked(old.region, -1)
		close(old.send) // replace a stale connection
	}
	h.conns[peerID] = c
	h.adjustRegionLocked("unknown", 1)
	h.mu.Unlock()

	h.log.Info("node connected", "peer_id", peerID)
	go c.writePump()
	c.readPump() // blocks until close

	h.mu.Lock()
	if h.conns[peerID] == c {
		h.adjustRegionLocked(c.region, -1)
		delete(h.conns, peerID)
	}
	h.mu.Unlock()
	_ = h.store.SetNodeStatus(context.Background(), peerID, "offline")
	h.cache.DelPrefix(context.Background(), "nodes:disco:")
	h.log.Info("node disconnected", "peer_id", peerID)
}

type conn struct {
	peerID string
	region string
	ws     *websocket.Conn
	send   chan []byte
	hub    *Hub
}

func (h *Hub) adjustRegionLocked(region string, delta int) {
	region = metrics.NormalizeRegion(region)
	h.regionCounts[region] += delta
	if h.regionCounts[region] <= 0 {
		delete(h.regionCounts, region)
		metrics.UpdateActiveNodeSessions(region, 0, h.environment)
		return
	}
	metrics.UpdateActiveNodeSessions(region, h.regionCounts[region], h.environment)
}

func (c *conn) setRegion(region string) {
	region = metrics.NormalizeRegion(region)
	if region == c.region {
		return
	}
	c.hub.mu.Lock()
	defer c.hub.mu.Unlock()
	c.hub.adjustRegionLocked(c.region, -1)
	c.region = region
	c.hub.adjustRegionLocked(c.region, 1)
}

func (c *conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.ws.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.ws.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *conn) readPump() {
	c.ws.SetReadLimit(1 << 20)
	_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(pongWait))
		c.handle(raw)
	}
}

func (c *conn) handle(raw []byte) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		c.hub.log.Warn("bad node frame", "peer_id", c.peerID, "err", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch env.Type {
	case TypeHello:
		c.onHello(ctx, env.Data)
	case TypeHeartbeat:
		c.onHeartbeat(ctx, env.Data)
	case TypeUsageReport:
		c.onUsage(ctx, env.Data)
	case TypeCommandResult:
		var res CommandResult
		_ = json.Unmarshal(env.Data, &res)
		c.hub.log.Info("command result", "peer_id", c.peerID, "request_id", res.RequestID, "ok", res.OK, "error", res.Error)
	default:
		c.hub.log.Debug("unknown node message", "type", env.Type) // ignore per protocol
	}
}

func (c *conn) onHello(ctx context.Context, data json.RawMessage) {
	var h Hello
	if err := json.Unmarshal(data, &h); err != nil {
		return
	}
	spec, _ := json.Marshal(h.Spec)
	caps, _ := json.Marshal(h.Capabilities)
	eps, _ := json.Marshal(h.Endpoints)
	if err := c.hub.store.ApplyHello(ctx, store.HelloUpdate{
		PeerID: c.peerID, IP: h.Spec.IP, IPHash: h.Identity.IPHash, Version: h.Version,
		Region: h.Spec.Region, Zone: h.Spec.Zone, AccessMode: normalizeAccessMode(h.Capabilities.AccessMode),
		Spec: spec, Capabilities: caps, Endpoints: eps,
		Protocols: protocolsFromEndpoints(h.Endpoints),
	}); err != nil {
		c.hub.log.Warn("apply hello failed", "peer_id", c.peerID, "err", err)
	}
	if h.DeploymentProfile != "" {
		if err := c.hub.store.UpdateOrgNodeDeploymentProfile(ctx, c.peerID, h.DeploymentProfile); err != nil {
			c.hub.log.Warn("update deployment profile failed", "peer_id", c.peerID, "err", err)
		}
	}
	if len(h.Services) > 0 {
		if err := c.hub.store.UpdateNodeServicesFromReport(ctx, c.peerID, h.Services); err != nil {
			c.hub.log.Warn("apply hello services failed", "peer_id", c.peerID, "err", err)
		}
	}
	c.setRegion(h.Spec.Region)
	if frame, err := wrap(TypeHelloAck, HelloAck{HeartbeatIntervalSec: heartbeatIntervalSec}); err == nil {
		select {
		case c.send <- frame:
		default:
		}
	}
}

func (c *conn) onHeartbeat(ctx context.Context, data json.RawMessage) {
	var hb Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return
	}
	status := hb.Status
	if status == "" {
		status = "online"
	}
	load, _ := json.Marshal(hb.Load)
	st, _ := json.Marshal(hb.Speedtest)
	if err := c.hub.store.ApplyHeartbeat(ctx, c.peerID, status, load, st,
		hb.Load.RxBytes, hb.Load.TxBytes, hb.Versions["node"]); err != nil {
		c.hub.log.Warn("apply heartbeat failed", "peer_id", c.peerID, "err", err)
		metrics.NodeHeartbeatsTotal.WithLabelValues("failed", c.hub.environment).Inc()
		return
	}
	c.hub.cache.DelPrefix(ctx, "nodes:disco:")
	_ = c.hub.store.TouchOrgNodeHeartbeat(ctx, c.peerID, time.Now())
	if len(hb.Services) > 0 {
		if err := c.hub.store.UpdateNodeServicesFromReport(ctx, c.peerID, hb.Services); err != nil {
			c.hub.log.Warn("apply heartbeat services failed", "peer_id", c.peerID, "err", err)
		}
	}
	metrics.NodeHeartbeatsTotal.WithLabelValues("success", c.hub.environment).Inc()
	// Time-series rollup for operator charts (per-minute bucket, last write wins).
	if internalID, err := c.hub.store.NodeInternalID(ctx, c.peerID); err == nil {
		if err := c.hub.store.RecordNodeMetrics(ctx, internalID, time.Now(),
			hb.Load.WGPeersRegistered, hb.Load.WGPeersConnected, hb.Load.ProxySessions, hb.Load.RxBytes, hb.Load.TxBytes,
			hb.Load.CPUPct, hb.Load.MemPct); err != nil {
			c.hub.log.Warn("record node metrics failed", "peer_id", c.peerID, "err", err)
		}
	}
}

// normalizeAccessMode returns a valid access mode, or "" to keep the prior value.
func normalizeAccessMode(m string) string {
	switch m {
	case "public", "private":
		return m
	default:
		return ""
	}
}

func (c *conn) onUsage(ctx context.Context, data json.RawMessage) {
	var ur UsageReport
	if err := json.Unmarshal(data, &ur); err != nil {
		return
	}
	for _, p := range ur.Peers {
		if err := c.hub.store.AddUsage(ctx, p.PeerID, p.RxBytesDelta, p.TxBytesDelta, p.LastHandshake); err != nil {
			c.hub.log.Warn("add usage failed", "client_id", p.PeerID, "err", err)
		}
	}
}

// protocolsFromEndpoints derives the advertised protocol list from the
// endpoints a node reports in its hello.
func protocolsFromEndpoints(e Endpoints) []string {
	out := []string{}
	if e.WireGuard.Port != 0 {
		out = append(out, "wireguard")
	}
	if e.VLESSReality.Port != 0 {
		out = append(out, "vless-reality")
	}
	if e.Hysteria2.Port != 0 {
		out = append(out, "hysteria2")
	}
	return out
}
