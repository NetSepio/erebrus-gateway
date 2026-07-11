package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
)

// APIKeyPrefix is the human-visible, non-secret prefix of every key.
const APIKeyPrefix = "erebrus_sk_"

// hashAPIKeyToken returns a deterministic digest for indexed DB lookup.
// Org API keys are 192-bit random tokens (crypto/rand), not user passwords.
func hashAPIKeyToken(token string) string {
	sum := sha256.Sum256([]byte(token)) // CodeQL[go/weak-sensitive-data-hashing]: high-entropy API key, not a password; SHA-256 is standard for token fingerprinting (indexed lookup).
	return hex.EncodeToString(sum[:])
}

// CreateAPIKey mints an org API key. The full secret is returned ONCE; only its
// sha256 hash and display prefix are stored.
func (s *Store) CreateAPIKey(ctx context.Context, orgID, name string) (secret string, key *APIKey, err error) {
	raw := make([]byte, 24)
	if _, err = rand.Read(raw); err != nil {
		return "", nil, err
	}
	secret = APIKeyPrefix + hex.EncodeToString(raw)
	hash := hashAPIKeyToken(secret)
	prefix := secret[:len(APIKeyPrefix)+6] // erebrus_sk_ + 6 chars

	var k APIKey
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO api_keys (org_id, name, prefix, key_hash)
		 VALUES ($1, NULLIF($2,''), $3, $4)
		 RETURNING id, org_id, COALESCE(name,''), prefix, created_at`,
		orgID, name, prefix, hash).
		Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.CreatedAt)
	if err != nil {
		return "", nil, err
	}
	return secret, &k, nil
}

// ListAPIKeys returns an org's keys (no secrets, no hashes).
func (s *Store) ListAPIKeys(ctx context.Context, orgID string) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, org_id, COALESCE(name,''), prefix, created_at, last_used_at, revoked_at
		 FROM api_keys WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey revokes a key within an org.
func (s *Store) RevokeAPIKey(ctx context.Context, orgID, keyID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = now() WHERE id=$1 AND org_id=$2 AND revoked_at IS NULL`,
		keyID, orgID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// LookupAPIKey resolves a presented secret to its org id, or ErrNotFound if the
// key is unknown or revoked. It also bumps last_used_at.
func (s *Store) LookupAPIKey(ctx context.Context, secret string) (orgID string, err error) {
	hash := hashAPIKeyToken(secret)
	err = s.db.QueryRowContext(ctx,
		`UPDATE api_keys SET last_used_at = now()
		 WHERE key_hash=$1 AND revoked_at IS NULL
		 RETURNING org_id`, hash).Scan(&orgID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return orgID, err
}
