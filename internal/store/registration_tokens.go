package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/secrets"
	"github.com/lib/pq"
)

func hashRegistrationToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Maximum lifetime of a node registration token (7 days).
const maxRegistrationTokenTTL = 7 * 24 * time.Hour

// CreateNodeRegistrationToken mints a scoped registration token for an org.
func (s *Store) CreateNodeRegistrationToken(ctx context.Context, orgID, createdBy, peerID string, scopes []string, expiresAt time.Time) (plain string, tok *NodeRegistrationToken, err error) {
	if len(scopes) == 0 {
		return "", nil, fmt.Errorf("at least one scope is required")
	}
	for _, sc := range scopes {
		if !isValidRegistrationScope(sc) {
			return "", nil, fmt.Errorf("invalid scope: %s", sc)
		}
	}
	now := time.Now()
	if !expiresAt.After(now) {
		return "", nil, fmt.Errorf("expires_at must be in the future")
	}
	if maxExpiresAt := now.Add(maxRegistrationTokenTTL); expiresAt.After(maxExpiresAt) {
		expiresAt = maxExpiresAt
	}
	plain, err = secrets.NewNodeRegistrationToken()
	if err != nil {
		return "", nil, err
	}
	hash := hashRegistrationToken(plain)
	var t NodeRegistrationToken
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO node_registration_tokens (org_id, token_hash, scopes, expires_at, peer_id, created_by)
		 VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,'')::uuid)
		 RETURNING id, org_id, peer_id, scopes, expires_at, created_by, used_at, revoked_at, created_at`,
		orgID, hash, pq.Array(scopes), expiresAt, peerID, createdBy).
		Scan(&t.ID, &t.OrgID, &t.PeerID, pq.Array(&t.Scopes), &t.ExpiresAt, &t.CreatedBy, &t.UsedAt, &t.RevokedAt, &t.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return plain, &t, nil
}

// LookupNodeRegistrationToken resolves a presented token to its org, optional peer, and scopes.
func (s *Store) LookupNodeRegistrationToken(ctx context.Context, token, requiredScope string) (orgID, tokenID, peerID string, scopes []string, err error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", "", nil, ErrNotFound
	}
	hash := hashRegistrationToken(token)
	var expiresAt time.Time
	var usedAt, revokedAt sql.NullTime
	err = s.db.QueryRowContext(ctx,
		`SELECT id, org_id, peer_id, scopes, expires_at, used_at, revoked_at
		 FROM node_registration_tokens WHERE token_hash=$1`, hash).
		Scan(&tokenID, &orgID, &peerID, pq.Array(&scopes), &expiresAt, &usedAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", nil, ErrNotFound
	}
	if err != nil {
		return "", "", "", nil, err
	}
	if revokedAt.Valid {
		return "", "", "", nil, ErrNotFound
	}
	if time.Now().After(expiresAt) {
		return "", "", "", nil, ErrNotFound
	}
	if requiredScope != "" && !scopeAllowed(scopes, requiredScope) {
		return "", "", "", nil, ErrNotFound
	}
	return orgID, tokenID, peerID, scopes, nil
}

// MarkNodeRegistrationTokenUsed records token consumption.
func (s *Store) MarkNodeRegistrationTokenUsed(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE node_registration_tokens SET used_at=now() WHERE id=$1 AND used_at IS NULL`, tokenID)
	return err
}

func isValidRegistrationScope(scope string) bool {
	switch scope {
	case TokenScopeNodeRegistration, TokenScopeFirewallSetup, TokenScopeServiceSetup:
		return true
	default:
		return false
	}
}

func scopeAllowed(scopes []string, required string) bool {
	for _, sc := range scopes {
		if sc == required {
			return true
		}
	}
	return false
}
