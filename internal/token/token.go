// Package token issues and verifies PASETO v4 (public) tokens for the three
// principal types: users, admins (a user claim with role=admin), and nodes.
package token

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/vk-rv/pvx"
)

// Roles.
const (
	RoleUser        = "user"
	RoleAdmin       = "admin"
	RoleNode        = "node"
	RoleGatewayCall = "gateway_call"
)

// Claims is the PASETO payload. Embeds pvx.RegisteredClaims (exp/iat/nbf) whose
// Valid() is promoted, so verification checks time-based validity automatically.
type Claims struct {
	UserID   string `json:"user_id,omitempty"`
	Wallet   string `json:"wallet,omitempty"`
	Chain    string `json:"chain,omitempty"`
	Role     string `json:"role,omitempty"`
	NodeID   string `json:"node_id,omitempty"`
	PeerID   string `json:"peer_id,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	SignedBy string `json:"signed_by,omitempty"`
	pvx.RegisteredClaims
}

// Manager signs and verifies tokens with one Ed25519 keypair.
type Manager struct {
	sk       *pvx.AsymSecretKey
	pk       *pvx.AsymPublicKey
	pub      ed25519.PublicKey
	pv4      *pvx.ProtoV4Public
	signedBy string
	ttl      time.Duration
}

// New builds a Manager from a hex-encoded Ed25519 private key (optionally
// "0x"-prefixed). The 64-byte ed25519 private key encodes its own public half.
func New(hexKey, signedBy string, ttl time.Duration) (*Manager, error) {
	raw, err := hex.DecodeString(strings.TrimPrefix(hexKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode paseto key: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("paseto key must be %d bytes, got %d", ed25519.PrivateKeySize, len(raw))
	}
	priv := ed25519.PrivateKey(raw)
	pub := priv.Public().(ed25519.PublicKey)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		sk:       pvx.NewAsymmetricSecretKey(priv, pvx.Version4),
		pk:       pvx.NewAsymmetricPublicKey(pub, pvx.Version4),
		pub:      pub,
		pv4:      pvx.NewPV4Public(),
		signedBy: signedBy,
		ttl:      ttl,
	}, nil
}

// IssueUser mints a user/admin token.
func (m *Manager) IssueUser(userID, wallet, chain, role string) (string, error) {
	if role == "" {
		role = RoleUser
	}
	return m.issue(Claims{UserID: userID, Wallet: wallet, Chain: chain, Role: role}, m.ttl)
}

// IssueNode mints a node token for the WS control plane.
func (m *Manager) IssueNode(nodeID, peerID string) (string, error) {
	return m.issue(Claims{NodeID: nodeID, PeerID: peerID, Role: RoleNode}, m.ttl)
}

// IssueGatewayCall mints a short-lived token for gateway→node private API calls.
func (m *Manager) IssueGatewayCall(nodeID, peerID, purpose string) (string, error) {
	if purpose == "" {
		purpose = "node_api"
	}
	return m.issue(Claims{NodeID: nodeID, PeerID: peerID, Role: RoleGatewayCall, Purpose: purpose}, 60*time.Second)
}

// PublicKeyHex returns the gateway Ed25519 public key as hex (nodes verify gateway calls).
func (m *Manager) PublicKeyHex() string {
	return hex.EncodeToString(m.pub)
}

func (m *Manager) issue(c Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	if ttl <= 0 {
		ttl = m.ttl
	}
	exp := now.Add(ttl)
	c.SignedBy = m.signedBy
	c.RegisteredClaims = pvx.RegisteredClaims{
		Issuer:     m.signedBy,
		IssuedAt:   &now,
		NotBefore:  &now,
		Expiration: &exp,
	}
	return m.pv4.Sign(m.sk, &c)
}

// Reconfigure updates issuer footer and TTL for newly minted tokens (admin settings).
func (m *Manager) Reconfigure(signedBy string, ttl time.Duration) {
	if signedBy != "" {
		m.signedBy = signedBy
	}
	if ttl > 0 {
		m.ttl = ttl
	}
}

// Verify parses and validates a token, returning its claims.
func (m *Manager) Verify(tokenString string) (*Claims, error) {
	var c Claims
	if err := m.pv4.Verify(tokenString, m.pk).ScanClaims(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
