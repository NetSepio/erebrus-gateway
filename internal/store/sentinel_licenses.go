package store

import (
	"context"
	"database/sql"
	"errors"
)

const sentinelLicenseCols = `id, org_id, COALESCE(node_id,''), status, source, created_at, updated_at`

func scanSentinelLicense(sc interface{ Scan(...any) error }) (*SentinelLicense, error) {
	var l SentinelLicense
	err := sc.Scan(&l.ID, &l.OrgID, &l.NodeID, &l.Status, &l.Source, &l.CreatedAt, &l.UpdatedAt)
	return &l, err
}

// CreateSentinelLicenses seeds included Sentinel licenses for an org.
func (s *Store) CreateSentinelLicenses(ctx context.Context, orgID string, count int, source string) error {
	for i := 0; i < count; i++ {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO sentinel_licenses (org_id, status, source) VALUES ($1, $2, $3)`,
			orgID, SentinelLicenseAvailable, source); err != nil {
			return err
		}
	}
	return nil
}

// CountAvailableSentinelLicenses returns unattached Sentinel licenses.
func (s *Store) CountAvailableSentinelLicenses(ctx context.Context, orgID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sentinel_licenses WHERE org_id=$1 AND status=$2`,
		orgID, SentinelLicenseAvailable).Scan(&n)
	return n, err
}

// AttachSentinelLicense binds an available license to a node.
func (s *Store) AttachSentinelLicense(ctx context.Context, orgID, nodeID string) (*SentinelLicense, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	var licID string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM sentinel_licenses
		 WHERE org_id=$1 AND status=$2 ORDER BY created_at LIMIT 1 FOR UPDATE`,
		orgID, SentinelLicenseAvailable).Scan(&licID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	lic, err := scanSentinelLicense(tx.QueryRowContext(ctx,
		`UPDATE sentinel_licenses SET node_id=$2, status=$3, updated_at=now() WHERE id=$1
		 RETURNING `+sentinelLicenseCols,
		licID, nodeID, SentinelLicenseAttached))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lic, nil
}

// ListSentinelLicenses returns licenses for an org.
func (s *Store) ListSentinelLicenses(ctx context.Context, orgID string) ([]SentinelLicense, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sentinelLicenseCols+` FROM sentinel_licenses WHERE org_id=$1 ORDER BY created_at`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SentinelLicense
	for rows.Next() {
		l, err := scanSentinelLicense(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}
