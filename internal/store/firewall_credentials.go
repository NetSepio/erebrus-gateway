package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FirewallCredential is a node's Shield (AdGuard Home) admin credential. Secret
// is the encrypted password blob (opaque; the API layer seals/opens it).
type FirewallCredential struct {
	NodeID    string
	AdminUser string
	Secret    []byte
	AdminURL  string
	UpdatedAt time.Time
}

// UpsertFirewallCredential stores (or replaces) a node's encrypted admin secret.
func (s *Store) UpsertFirewallCredential(ctx context.Context, nodeID, adminUser string, secret []byte, adminURL string) error {
	if adminUser == "" {
		adminUser = "admin"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_firewall_credentials (node_id, admin_user, admin_secret, admin_url)
		 VALUES ($1, $2, $3, NULLIF($4,''))
		 ON CONFLICT (node_id) DO UPDATE SET
			admin_user = EXCLUDED.admin_user,
			admin_secret = EXCLUDED.admin_secret,
			admin_url = COALESCE(NULLIF(EXCLUDED.admin_url,''), node_firewall_credentials.admin_url),
			updated_at = now()`,
		nodeID, adminUser, secret, adminURL)
	return err
}

// GetFirewallCredential returns a node's encrypted admin credential.
func (s *Store) GetFirewallCredential(ctx context.Context, nodeID string) (*FirewallCredential, error) {
	var c FirewallCredential
	var url sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT node_id, admin_user, admin_secret, admin_url, updated_at
		 FROM node_firewall_credentials WHERE node_id = $1`, nodeID).
		Scan(&c.NodeID, &c.AdminUser, &c.Secret, &url, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	c.AdminURL = url.String
	return &c, nil
}
