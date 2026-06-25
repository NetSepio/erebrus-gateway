package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// FlowID is a pending wallet-signature challenge.
type FlowID struct {
	FlowID        string
	WalletAddress string
	Chain         string
	ExpiresAt     time.Time
}

// CreateFlowID stores a login challenge.
func (s *Store) CreateFlowID(ctx context.Context, flowID, wallet, chain string, ttl time.Duration) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO flow_ids (flow_id, wallet_address, chain, expires_at)
		 VALUES ($1, $2, $3, $4)`,
		flowID, wallet, chain, time.Now().Add(ttl))
	return err
}

// GetFlowID returns a challenge if it exists and has not expired.
func (s *Store) GetFlowID(ctx context.Context, flowID string) (*FlowID, error) {
	var f FlowID
	err := s.db.QueryRowContext(ctx,
		`SELECT flow_id, wallet_address, chain, expires_at FROM flow_ids WHERE flow_id = $1`, flowID).
		Scan(&f.FlowID, &f.WalletAddress, &f.Chain, &f.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if time.Now().After(f.ExpiresAt) {
		_ = s.DeleteFlowID(ctx, flowID)
		return nil, ErrNotFound
	}
	return &f, nil
}

// DeleteFlowID removes a challenge (single-use).
func (s *Store) DeleteFlowID(ctx context.Context, flowID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM flow_ids WHERE flow_id = $1`, flowID)
	return err
}

// PurgeExpiredFlowIDs deletes stale challenges (called periodically).
func (s *Store) PurgeExpiredFlowIDs(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM flow_ids WHERE expires_at < now()`)
	return err
}
