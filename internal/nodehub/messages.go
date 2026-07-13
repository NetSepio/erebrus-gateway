// Package nodehub implements the gateway side of the node↔gateway control
// plane: HTTPS registration, the WebSocket hub, and persistence of node status,
// heartbeats and per-client usage.
//
// The message structs below are a hand-mirrored copy of docs/ws-protocol.md
// (FROZEN v2.0). The node repo carries the same structs in
// erebrus/internal/gatewayclient/messages.go; both sides have contract tests
// that marshal the canonical examples. Change ws-protocol.md first, then both.
package nodehub

import "encoding/json"

// Message types.
const (
	TypeHello         = "hello"
	TypeHelloAck      = "hello_ack"
	TypeHeartbeat     = "heartbeat"
	TypeUsageReport   = "usage_report"
	TypeCommand       = "command"
	TypeCommandResult = "command_result"
)

// Command actions (v2.0).
const (
	ActionDrain                    = "drain"
	ActionUndrain                  = "undrain"
	ActionRotateReality            = "rotate_reality"
	ActionResyncPeers              = "resync_peers"
	ActionSyncFirewall             = "sync_firewall"
	ActionRestartFirewall          = "restart_firewall"
	ActionResetFirewallCredentials = "reset_firewall_credentials"
	ActionSetFirewallCredentials   = "set_firewall_credentials"
)

// Envelope wraps every WebSocket frame: {"type": "...", "data": {...}}.
type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Identity is the node's stable identity anchor.
type Identity struct {
	PeerID string `json:"peer_id"`
	DID    string `json:"did"`
	IPHash string `json:"ip_hash"`
}

// Spec is coarse node hardware/placement.
type Spec struct {
	CPU    string `json:"cpu"`
	MemMB  int    `json:"mem_mb"`
	Region string `json:"region"`
	Zone   string `json:"zone,omitempty"`
	IP     string `json:"ip"`
}

// Capabilities advertises optional node features.
type Capabilities struct {
	AccessMode string          `json:"access_mode,omitempty"` // private | public
	Drop       *DropCapability `json:"drop,omitempty"`        // additive; absent on non-Drop nodes
}

// DropCapability advertises a node's optional Drop (Kubo storage) support. It is
// additive and omitted entirely by nodes that do not run Drop, so older nodes
// remain contract-compatible. Mirrors the erebrus node contract exactly.
type DropCapability struct {
	Enabled              bool `json:"enabled"`
	AcceptsPublicUploads bool `json:"accepts_public_uploads"`
	WebUIAvailable       bool `json:"webui_available"`
}

// DropStatus is the node's Drop runtime health and capacity, reported in each
// heartbeat. Additive and omitted by non-Drop nodes. Mirrors the erebrus node
// contract exactly. State is one of:
// disabled | starting | active | degraded | full | unreachable.
type DropStatus struct {
	State           string `json:"state"`
	KuboVersion     string `json:"kubo_version"`
	RepoSizeBytes   int64  `json:"repo_size_bytes"`
	StorageMaxBytes int64  `json:"storage_max_bytes"`
	NumObjects      int64  `json:"num_objects"`
}

// Endpoints describes the connection endpoints clients dial.
type Endpoints struct {
	WireGuard    WireGuardEndpoint `json:"wireguard"`
	VLESSReality VLESSEndpoint     `json:"vless_reality"`
	Hysteria2    Hysteria2Endpoint `json:"hysteria2"`
}

type WireGuardEndpoint struct {
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port"`
	PublicKey string `json:"public_key"`
}

type VLESSEndpoint struct {
	Port      int      `json:"port"`
	PublicKey string   `json:"public_key"`
	ShortIDs  []string `json:"short_ids"`
	SNI       string   `json:"sni"`
}

type Hysteria2Endpoint struct {
	Port int    `json:"port"`
	Obfs string `json:"obfs"`
}

// Hello is sent by the node on every (re)connect.
type Hello struct {
	NodeID            string            `json:"node_id"`
	Version           string            `json:"version"`
	Identity          Identity          `json:"identity"`
	Spec              Spec              `json:"spec"`
	Capabilities      Capabilities      `json:"capabilities"`
	Endpoints         Endpoints         `json:"endpoints"`
	DeploymentProfile string            `json:"deployment_profile,omitempty"`
	Services          map[string]string `json:"services,omitempty"`
}

// HelloAck is the gateway's response to hello.
type HelloAck struct {
	HeartbeatIntervalSec int `json:"heartbeat_interval_sec"`
}

// Load is the node's coarse load snapshot.
type Load struct {
	WGPeersRegistered int     `json:"wg_peers_registered"`
	WGPeersConnected  int     `json:"wg_peers_connected"`
	WGPeers           int     `json:"wg_peers,omitempty"` // legacy field from old nodes
	ProxySessions     int     `json:"proxy_sessions"`
	CPUPct            float64 `json:"cpu_pct"`
	MemPct            float64 `json:"mem_pct"`
	RxBytes           int64   `json:"rx_bytes"`
	TxBytes           int64   `json:"tx_bytes"`
}

// Speedtest is the node's most recent self-measured throughput.
type Speedtest struct {
	DownloadMbps float64 `json:"download_mbps"`
	UploadMbps   float64 `json:"upload_mbps"`
	LatencyMs    float64 `json:"latency_ms"`
	MeasuredAt   int64   `json:"measured_at"`
}

// Heartbeat is sent every heartbeat_interval_sec.
type Heartbeat struct {
	TS        int64             `json:"ts"`
	Status    string            `json:"status"` // online | draining
	Load      Load              `json:"load"`
	Speedtest Speedtest         `json:"speedtest"`
	Versions  map[string]string `json:"versions"` // may carry "kubo" on Drop nodes
	Services  map[string]string `json:"services,omitempty"`
	Drop      *DropStatus       `json:"drop,omitempty"` // additive; absent on non-Drop nodes
}

// PeerUsage is one client's traffic delta in a usage_report.
type PeerUsage struct {
	PeerID        string `json:"peer_id"`
	RxBytesDelta  int64  `json:"rx_bytes_delta"`
	TxBytesDelta  int64  `json:"tx_bytes_delta"`
	LastHandshake int64  `json:"last_handshake"`
}

// UsageReport is sent every 60s with per-client deltas.
type UsageReport struct {
	TS    int64       `json:"ts"`
	Peers []PeerUsage `json:"peers"`
}

// Command is gateway → node.
type Command struct {
	Action    string          `json:"action"`
	RequestID string          `json:"request_id"`
	Args      json.RawMessage `json:"args,omitempty"`
}

// CommandResult is node → gateway.
type CommandResult struct {
	RequestID string `json:"request_id"`
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
}

// wrap marshals a typed payload into an Envelope frame.
func wrap(msgType string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{Type: msgType, Data: data})
}
