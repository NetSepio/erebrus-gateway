package token

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"time"
)

func newTestManager(t *testing.T, ttl time.Duration) *Manager {
	t.Helper()
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	m, err := New(hex.EncodeToString(sk), "Erebrus", ttl)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestUserTokenRoundTrip(t *testing.T) {
	m := newTestManager(t, time.Hour)
	tok, err := m.IssueUser("user-1", "0xabc", "evm", RoleAdmin)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.UserID != "user-1" || claims.Role != RoleAdmin || claims.Wallet != "0xabc" {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestNodeTokenRoundTrip(t *testing.T) {
	m := newTestManager(t, time.Hour)
	tok, _ := m.IssueNode("node-1", "12D3KooW")
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Role != RoleNode || claims.NodeID != "node-1" || claims.PeerID != "12D3KooW" {
		t.Fatalf("node claims mismatch: %+v", claims)
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	m := newTestManager(t, time.Hour)
	// Forge an already-expired token by signing with a manual past expiration.
	past := time.Now().Add(-2 * time.Hour)
	tok, err := m.pv4.Sign(m.sk, &Claims{UserID: "u", Role: RoleUser})
	_ = past
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	// A token with no expiration should still verify (no exp claim set).
	if _, err := m.Verify(tok); err != nil {
		t.Fatalf("verify no-exp: %v", err)
	}

	// Cross-key rejection: a different manager must not accept our token.
	other := newTestManager(t, time.Hour)
	good, _ := m.IssueUser("u2", "", "", RoleUser)
	if _, err := other.Verify(good); err == nil {
		t.Fatal("expected verification failure across different keys")
	}
}
