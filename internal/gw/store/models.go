package store

import (
	"encoding/json"
	"time"
)

// User is a gateway account, keyed by wallet (or email for social/email login).
type User struct {
	ID            string    `json:"id"`
	WalletAddress string    `json:"wallet_address,omitempty"`
	Chain         string    `json:"chain,omitempty"`
	Role          string    `json:"role"`
	Email         string    `json:"email,omitempty"`
	Name          string    `json:"name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Node is a registered VPN node and its latest control-plane snapshot.
type Node struct {
	ID            string          `json:"id"`
	PeerID        string          `json:"peer_id"`
	DID           string          `json:"did"`
	WalletAddress string          `json:"wallet_address,omitempty"`
	Name          string          `json:"name"`
	Region        string          `json:"region"`
	IP            string          `json:"ip,omitempty"` // never serialized publicly
	IPHash        string          `json:"ip_hash,omitempty"`
	Spec          json.RawMessage `json:"spec"`
	Capabilities  json.RawMessage `json:"capabilities"`
	Endpoints     json.RawMessage `json:"endpoints"`
	Protocols     []string        `json:"protocols"`
	Status        string          `json:"status"`
	Load          json.RawMessage `json:"load"`
	Speedtest     json.RawMessage `json:"speedtest"`
	RxBytes       int64           `json:"rx_bytes"`
	TxBytes       int64           `json:"tx_bytes"`
	Version       string          `json:"version"`
	LastHeartbeat *time.Time      `json:"last_heartbeat,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Plan is a subscription tier.
type Plan struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PriceUSDC  string `json:"price_usdc"`
	PeriodDays int    `json:"period_days"`
	MaxClients int    `json:"max_clients"`
}

// Subscription is a user's (or org's) entitlement.
type Subscription struct {
	ID               string     `json:"id"`
	PlanID           string     `json:"plan_id"`
	Source           string     `json:"source"`
	Status           string     `json:"status"`
	CurrentPeriodEnd *time.Time `json:"current_period_end,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Client is a provisioned VPN client.
type Client struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	OrgID         string     `json:"org_id,omitempty"`
	NodeID        string     `json:"node_id"`
	Name          string     `json:"name"`
	WGPublicKey   string     `json:"wg_public_key"`
	WGAllowedIP   string     `json:"wg_allowed_ip,omitempty"`
	Status        string     `json:"status"`
	RxBytes       int64      `json:"rx_bytes"`
	TxBytes       int64      `json:"tx_bytes"`
	LastHandshake *time.Time `json:"last_handshake,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}
